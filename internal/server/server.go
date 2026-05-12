package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openbiss/openbiss/internal/config"
	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/pkcs11"
	"github.com/openbiss/openbiss/internal/server/openapi"
	loctls "github.com/openbiss/openbiss/internal/tls"
	ui "github.com/openbiss/openbiss/internal/ui"
)

// ServerState represents the lifecycle state of the Server.
type ServerState int32

const (
	StateStopped  ServerState = 0
	StateStarting ServerState = 1
	StateRunning  ServerState = 2
	StateStopping ServerState = 3
)

// String returns the string representation of ServerState.
func (s ServerState) String() string {
	switch s {
	case StateStopped:
		return "Stopped"
	case StateStarting:
		return "Starting"
	case StateRunning:
		return "Running"
	case StateStopping:
		return "Stopping"
	default:
		return "Unknown"
	}
}

// Server is the OpenBISS HTTPS server. It manages TLS, port selection,
// routing, and PKCS#11 driver lifecycle.
type Server struct {
	cfg               *config.Config
	logger            *slog.Logger
	dialog            ui.DialogProvider
	http              *http.Server
	port              atomic.Int32
	state             atomic.Int32
	startedAtUnixNano atomic.Int64

	// driver is the cached PKCS#11 driver. Loaded once in Start(), closed in
	// shutdown. Access through Driver() / ReloadDriver() — never directly.
	driver   *pkcs11.Driver
	driverMu sync.RWMutex

	stats RequestStats
}

// New creates a Server but does not start it. Call Start to begin accepting
// connections.
func New(cfg *config.Config, logger *slog.Logger, dialog ui.DialogProvider) (*Server, error) {
	return &Server{cfg: cfg, logger: logger, dialog: dialog}, nil
}

// Start binds to the first available BISS port (53952-53955), starts the HTTPS
// server, and blocks until ctx is cancelled or the server encounters a fatal error.
// A graceful shutdown is performed when ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	// Set state to Starting before attempting to bind.
	s.state.Store(int32(StateStarting))

	// Load or generate the self-signed localhost TLS certificate.
	cert, err := loctls.LoadOrGenerate(s.cfg.TLSCertPath(), s.cfg.TLSKeyPath())
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("server.tls_setup_failed"), err)
	}

	tlsCfg := loctls.NewTLSConfig(cert)

	listener, port, err := bindFirstAvailablePort(tlsCfg)
	if err != nil {
		return errors.New(i18n.T("server.no_port_available", config.BISSPorts))
	}

	// Store the port after successful bind.
	s.port.Store(int32(port))

	if d, err := s.loadDriver(); err != nil {
		slog.Warn("PKCS#11 driver not loaded at startup; will retry on first request", "error", err)
	} else {
		s.driverMu.Lock()
		s.driver = d
		s.driverMu.Unlock()
	}

	mux := s.buildMux()
	s.http = &http.Server{
		Handler:           s.statsMiddleware(corsMiddleware(mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          log.New(newSlogWriter(s.logger), "", 0),
	}

	slog.Info(i18n.T("app.listening"), "addr", fmt.Sprintf("https://127.0.0.1:%d", port))

	// Start serving in a goroutine so we can also watch ctx.
	errCh := make(chan error, 1)
	go func() {
		// Set state to Running and record start time.
		s.state.Store(int32(StateRunning))
		s.startedAtUnixNano.Store(time.Now().UnixNano())

		if err := s.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// Set state to Stopping before shutdown.
		s.state.Store(int32(StateStopping))

		// Extract and nil the driver under lock, then close outside the lock.
		// This prevents holding driverMu during Driver.Close() → ctx.Finalize(),
		// which itself acquires the driver's internal mutex (d.mu).
		s.driverMu.Lock()
		d := s.driver
		s.driver = nil
		s.driverMu.Unlock()
		if d != nil {
			d.Close()
		}

		slog.Info(i18n.T("app.shutting_down"))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := s.http.Shutdown(shutdownCtx)

		// After shutdown completes, reset state and port.
		s.state.Store(int32(StateStopped))
		s.port.Store(0)
		s.startedAtUnixNano.Store(0)

		return err
	}
}

// buildMux registers all BISS-compatible routes.
func (s *Server) buildMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/version", handleVersion)
	mux.HandleFunc("/getsigner", s.handleGetSigner)
	mux.HandleFunc("/sign", s.handleSign)

	mux.HandleFunc("/openapi.json", handleOpenAPIJSON)

	swaggerHandler := http.FileServer(http.FS(openapi.SwaggerUIFS()))
	mux.Handle("/docs/", http.StripPrefix("/docs/", swaggerHandler))
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusFound)
	})

	return mux
}

func handleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	spec, err := openapi.SpecJSON()
	if err != nil {
		http.Error(w, "openapi spec unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(spec)
}

// loadDriver attempts to load a PKCS#11 shared library in priority order.
// It tries each library path returned by discovery and returns the first one
// that initialises successfully. This handles the case where multiple smart
// card middleware packages are installed.
func (s *Server) loadDriver() (*pkcs11.Driver, error) {
	libs := pkcs11.DiscoverLibraries(s.cfg.PKCS11Lib)
	if len(libs) == 0 {
		return nil, errors.New(i18n.T("pkcs11.no_libraries"))
	}

	var lastErr error
	for _, lib := range libs {
		slog.Debug("trying PKCS#11 library", "lib", lib)
		d, err := pkcs11.NewDriver(lib)
		if err == nil {
			return d, nil
		}
		slog.Debug("PKCS#11 library failed", "lib", lib, "error", err)
		lastErr = err
	}

	return nil, errors.New(i18n.T("pkcs11.all_failed", lastErr))
}

// bindFirstAvailablePort tries each BISS port in order and returns the first
// TLS listener that successfully binds.
func bindFirstAvailablePort(tlsCfg *tls.Config) (net.Listener, int, error) {
	for _, port := range config.BISSPorts {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := tls.Listen("tcp", addr, tlsCfg)
		if err == nil {
			return ln, port, nil
		}
		slog.Debug("port unavailable", "port", port, "error", err)
	}
	return nil, 0, fmt.Errorf("ports %v all in use", config.BISSPorts)
}

// Driver returns the cached PKCS#11 driver, or nil if not loaded.
func (s *Server) Driver() *pkcs11.Driver {
	s.driverMu.RLock()
	defer s.driverMu.RUnlock()
	return s.driver
}

// ReloadDriver closes the existing driver (if any) and loads a fresh one.
// On error the old driver is already closed and s.driver is nil.
func (s *Server) ReloadDriver() error {
	d, err := s.loadDriver()

	s.driverMu.Lock()
	old := s.driver
	s.driver = d
	s.driverMu.Unlock()

	if old != nil {
		old.Close()
	}
	return err
}

// Port returns the current port the server is listening on, or 0 if stopped.
func (s *Server) Port() int {
	return int(s.port.Load())
}

// State returns the current state of the server.
func (s *Server) State() ServerState {
	return ServerState(s.state.Load())
}

// StartedAt returns the time the server started, or zero time if stopped.
func (s *Server) StartedAt() time.Time {
	nanos := s.startedAtUnixNano.Load()
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}

// Stats returns the request statistics tracker. Used by the GUI's API
// tab to read per-endpoint counters and the recent-requests ring.
func (s *Server) Stats() *RequestStats {
	return &s.stats
}

// Uptime returns the duration the server has been running, or 0 if stopped.
func (s *Server) Uptime() time.Duration {
	nanos := s.startedAtUnixNano.Load()
	if nanos == 0 {
		return 0
	}
	return time.Since(time.Unix(0, nanos))
}

type slogWriter struct {
	logger *slog.Logger
}

func newSlogWriter(logger *slog.Logger) *slogWriter {
	return &slogWriter{logger: logger}
}

func (w *slogWriter) Write(p []byte) (int, error) {
	w.logger.Warn(string(p))
	return len(p), nil
}

// statsMiddleware records each request's path, status code, and duration
// to s.stats. Wraps the response writer to capture WriteHeader calls.
// Mounted outermost so it sees the final status code AFTER cors + handlers.
func (s *Server) statsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(sr, r)
		s.stats.RecordRequest(r.Method, r.URL.Path, sr.statusCode, time.Since(start))
	})
}

// statusResponseWriter is a minimal http.ResponseWriter wrapper that
// captures the status code so middleware can record it post-handler.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusResponseWriter) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}
