// Package gui provides the Fyne-based desktop GUI for OpenBISS: the
// tray-resident application that hosts the HTTPS signing server, a main
// settings window, a first-run wizard, and the PIN/certificate dialogs.
//
// This package is imported only from the GUI code path; headless / CLI
// builds (server only) must not depend on it so the Fyne runtime is not
// linked into non-GUI binaries.
package gui

import (
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
// screens (wizard, settings, tray) can resolve paths without re-loading.
//
// The App does NOT own the server.Server lifecycle — main.go (T18) starts
// and stops the server. App holds a reference (set via SetServer) so the
// Status screen and other read-only views can poll the server's atomic
// state, port, driver, and uptime at 1 Hz.
type App struct {
	fyneApp fyne.App
	window  fyne.Window
	cfg     *config.Config
	srv     *server.Server
	tap     *logging.Tap
	useTray bool
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
