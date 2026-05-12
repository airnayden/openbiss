// Package gui provides the Fyne-based desktop GUI for OpenBISS: the
// desktop application that hosts the HTTPS signing server, a main
// settings window, a first-run wizard, and the PIN/certificate dialogs.
//
// This package is imported only from the GUI code path; headless / CLI
// builds (server only) must not depend on it so the Fyne runtime is not
// linked into non-GUI binaries.
package gui

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/openbiss/openbiss/assets"
	"github.com/openbiss/openbiss/internal/config"
	"github.com/openbiss/openbiss/internal/logging"
	"github.com/openbiss/openbiss/internal/server"
)

// AppID is the canonical Fyne app ID (reverse-DNS bundle identifier) used
// by app.NewWithID and mirrored in FyneApp.toml. Stable across platforms
// because Fyne uses it for per-app preference storage paths.
const AppID = "com.openbiss.openbiss"

// App is the OpenBISS Fyne GUI root. It owns the Fyne app handle and the
// main window, and holds a reference to the loaded config so downstream
// screens (wizard, settings) can resolve paths without re-loading.
//
// App owns the server lifecycle via StartServer / StopServer so the Status
// screen can start and stop the server through the StatusHost interface
// without importing internal/gui (which would create an import cycle).
type App struct {
	fyneApp fyne.App
	window  fyne.Window
	cfg     *config.Config
	srv     *server.Server
	tap     *logging.Tap

	serverMu     sync.Mutex
	serverCancel context.CancelFunc // nil when stopped
}

// New constructs the Fyne application, sets the app icon from the embedded
// PNG, and returns an App ready for window construction in later tasks.
// It does NOT show any window — callers must do that explicitly so headless
// startup paths (autostart, --no-gui flag) can skip window creation.
func New(cfg *config.Config) (*App, error) {
	fyneApp := app.NewWithID(AppID)

	if len(assets.IconPNG) > 0 {
		fyneApp.SetIcon(fyne.NewStaticResource("icon.png", assets.IconPNG))
	}

	return &App{
		fyneApp: fyneApp,
		cfg:     cfg,
	}, nil
}

// FyneApp returns the underlying fyne.App. Used by tray (T15) and
// downstream screens that need to construct windows or read preferences.
func (a *App) FyneApp() fyne.App {
	return a.fyneApp
}

// MainWindow returns the main settings window. Nil until T14 creates it.
func (a *App) MainWindow() fyne.Window {
	return a.window
}

// SetServer wires the server.Server reference into the App. main.go (T18)
// calls this after constructing the server and BEFORE BuildMainWindow so
// the Status screen can poll srv on first render. Passing nil is allowed
// but downstream screens that depend on the server will render placeholders.
func (a *App) SetServer(srv *server.Server) {
	a.srv = srv
}

// Server returns the wired server reference, or nil if SetServer has not
// yet been called.
func (a *App) Server() *server.Server {
	return a.srv
}

// Cfg returns the loaded configuration. Exposed so tab screens in the
// internal/gui/screens subpackage (e.g. Settings) can read and mutate the
// active config without an import cycle on internal/gui.
func (a *App) Cfg() *config.Config {
	return a.cfg
}

// SetTap wires the logging.Tap reference into the App so the Logs screen
// (T21) can subscribe to the slog ring buffer. main.go calls this after
// constructing the Tap and BEFORE BuildMainWindow so the Logs tab can
// subscribe on first render. Passing nil is allowed; downstream screens
// render placeholders when the tap is unavailable (test harnesses).
func (a *App) SetTap(tap *logging.Tap) {
	a.tap = tap
}

// Tap returns the wired logging tap reference, or nil if SetTap has not
// yet been called.
func (a *App) Tap() *logging.Tap {
	return a.tap
}

// StartServer starts the server in a background goroutine. Idempotent:
// calling while already running is a no-op and returns nil.
func (a *App) StartServer() error {
	if a.srv == nil {
		return errors.New("server not wired into App")
	}
	a.serverMu.Lock()
	if a.serverCancel != nil {
		a.serverMu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.serverCancel = cancel
	a.serverMu.Unlock()

	go func() {
		if err := a.srv.Start(ctx); err != nil {
			slog.Error("server: start returned error", "error", err)
		}
		a.serverMu.Lock()
		a.serverCancel = nil
		a.serverMu.Unlock()
	}()
	return nil
}

// StopServer requests a graceful shutdown. Idempotent: no-op when already
// stopped. Blocks until StateStopped or a 6-second timeout elapses.
func (a *App) StopServer() {
	a.serverMu.Lock()
	cancel := a.serverCancel
	a.serverCancel = nil
	a.serverMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	if a.srv == nil {
		return
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if a.srv.State() == server.StateStopped {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// IsServerRunning reports whether the server is in a non-stopped state
// (Starting, Running, or Stopping). Used by the Status screen to
// enable/disable Start/Stop buttons.
func (a *App) IsServerRunning() bool {
	if a.srv == nil {
		return false
	}
	return a.srv.State() != server.StateStopped
}
