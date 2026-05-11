package gui

import (
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/openbiss/openbiss/assets"
	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/instance"
)

// BuildTray sets up the system tray icon, menu, and window-close behaviour.
//
// Graceful degradation cascade:
//  1. Type-assert fyne.App to desktop.App; mobile/headless drivers fail this.
//  2. On Linux, probe for an active StatusNotifierWatcher D-Bus service —
//     GNOME without the AppIndicator extension lacks one and would simply
//     hide the tray icon, leaving the user with no way to interact.
//  3. Either step failing flips useTray=false: the close button becomes a
//     full Quit instead of a minimize-to-tray.
//
// Must be called after BuildMainWindow (a.window must be non-nil).
func (a *App) BuildTray() {
	if a.window == nil {
		slog.Warn("BuildTray called before BuildMainWindow; skipping")
		return
	}

	desk, ok := a.fyneApp.(desktop.App)
	if !ok {
		a.useTray = false
		a.applyCloseIntercept()
		slog.Warn(i18n.T("ui.error.tray_unavailable"))
		return
	}

	if !hasStatusNotifierWatcher() {
		a.useTray = false
		a.applyCloseIntercept()
		slog.Warn(i18n.T("ui.error.tray_unavailable"))
		return
	}

	a.useTray = true

	if len(assets.TrayLightPNG) > 0 {
		desk.SetSystemTrayIcon(fyne.NewStaticResource("tray-light.png", assets.TrayLightPNG))
	}

	statusLabel := fyne.NewMenuItem(i18n.T("ui.status.stopped"), nil)
	statusLabel.Disabled = true

	menu := fyne.NewMenu("OpenBISS",
		fyne.NewMenuItem(i18n.T("ui.tray.show"), func() {
			fyne.Do(func() {
				a.window.Show()
				a.window.RequestFocus()
			})
		}),
		statusLabel,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("ui.tray.quit"), func() {
			a.fyneApp.Quit()
		}),
	)
	desk.SetSystemTrayMenu(menu)

	instance.OnRaiseWindow(func() {
		fyne.Do(func() {
			a.window.Show()
			a.window.RequestFocus()
		})
	})

	a.applyCloseIntercept()
}

// applyCloseIntercept wires the window's close button: minimize-to-tray when
// the tray is active, full Quit otherwise. Splitting this out keeps the two
// degradation branches in BuildTray symmetric.
func (a *App) applyCloseIntercept() {
	if a.useTray {
		a.window.SetCloseIntercept(func() {
			a.window.Hide()
		})
		return
	}
	a.window.SetCloseIntercept(func() {
		a.fyneApp.Quit()
	})
}
