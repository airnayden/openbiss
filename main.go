// OpenBISS - Open-source replacement for BORICA BISS (Browser Independent Signing Service).
//
// BISS is a closed-source Java application used in Bulgaria's health system (НЗИС),
// e-prescriptions, and dental reporting. It runs as a local HTTPS server on ports
// 53952-53955 and allows browsers to sign documents using smart cards (КЕП/qualified
// electronic signatures) via PKCS#11.
//
// OpenBISS reimplements the same HTTP API with:
//   - Full BISS JSON API compatibility (/version, /getsigner, /sign)
//   - PKCS#11 smart card integration via miekg/pkcs11 (no cgo required)
//   - OS certificate store validation (NOT a custom trust store like BISS)
//   - Self-signed localhost TLS certificate generated on first run
//   - Native OS dialogs for PIN entry and certificate selection
//   - Port scanning across 53952-53955 (same as BISS)
package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openbiss/openbiss/internal/config"
	"github.com/openbiss/openbiss/internal/gui"
	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/instance"
	"github.com/openbiss/openbiss/internal/logging"
	"github.com/openbiss/openbiss/internal/server"
	"github.com/openbiss/openbiss/internal/ui"
	uifyne "github.com/openbiss/openbiss/internal/ui/fyne"
)

func main() {
	headlessFlag := flag.Bool("headless", false, "Run without GUI: server only, no window or tray")
	langFlag := flag.String("lang", "", "UI language: 'en' or 'bg' (overrides OPENBISS_LANG and OS locale)")
	flag.Parse()

	if *langFlag != "" {
		_ = os.Setenv("OPENBISS_LANG", *langFlag)
	}

	i18n.Init(i18n.DetectLanguage())

	slog.Info(i18n.T("app.starting"), "version", config.Version)

	cfg, err := config.Load()
	if err != nil {
		slog.Error(i18n.T("app.failed_load_config"), "error", err)
		log.Fatal(err)
	}

	i18n.Init(cfg.Lang)

	logLevel := logging.ParseLevel(cfg.LogLevel)
	tap, err := logging.NewTap(cfg, logLevel)
	if err != nil {
		log.Fatal(err)
	}
	slog.SetDefault(slog.New(tap))

	if *headlessFlag {
		runHeadless(cfg)
		return
	}

	runGUI(cfg, tap)
}

func runHeadless(cfg *config.Config) {
	srv, err := server.New(cfg, slog.Default(), ui.NewNative())
	if err != nil {
		slog.Error(i18n.T("app.failed_create_server"), "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(ctx); err != nil {
		slog.Error(i18n.T("app.server_error"), "error", err)
		os.Exit(1)
	}

	slog.Info(i18n.T("app.stopped"))
}

func runGUI(cfg *config.Config, tap *logging.Tap) {
	// CRITICAL Fyne ordering (panics if violated — see T14 learnings):
	// gui.New (app.NewWithID) → SetTap → BuildMainWindow → TryAcquire
	// → server.New → srv.Start goroutine → MaybeShowWizard goroutine → Run()
	// Run() blocks the OS main thread; server and wizard run in goroutines.
	// SetTap MUST precede BuildMainWindow so the Logs tab subscribes on first
	// render rather than the nil-tap placeholder.
	guiApp, err := gui.New(cfg)
	if err != nil {
		slog.Error("gui: failed to create app", "error", err)
		os.Exit(1)
	}

	guiApp.SetTap(tap)

	guiApp.BuildMainWindow()

	release, alreadyRunning, err := instance.TryAcquire(cfg.DataDir)
	if err != nil {
		slog.Error("instance: lock failed", "error", err)
		os.Exit(1)
	}
	if alreadyRunning {
		slog.Info("instance: another OpenBISS is running; requesting raise")
		os.Exit(0)
	}
	defer release()

	provider := uifyne.New(guiApp.FyneApp(), guiApp.MainWindow())

	srv, err := server.New(cfg, slog.Default(), provider)
	if err != nil {
		slog.Error(i18n.T("app.failed_create_server"), "error", err)
		os.Exit(1)
	}

	guiApp.SetServer(srv)

	if err := guiApp.StartServer(); err != nil {
		slog.Error(i18n.T("app.failed_create_server"), "error", err)
		os.Exit(1)
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		guiApp.MaybeShowWizard()
	}()

	guiApp.FyneApp().Lifecycle().SetOnStopped(func() {
		guiApp.StopServer()
	})

	guiApp.FyneApp().Run()

	time.Sleep(time.Second)
}
