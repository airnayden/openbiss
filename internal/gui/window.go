package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"github.com/openbiss/openbiss/internal/config"
	"github.com/openbiss/openbiss/internal/gui/screens"
	"github.com/openbiss/openbiss/internal/i18n"
)

var defaultWindowSize = fyne.NewSize(800, 600)

// BuildMainWindow constructs the main settings/status window with the
// five-tab layout (Status, Settings, Logs, Certificates, About), stores
// it on the App, and calls Show() so the window appears once the Fyne
// event loop starts. The close button is intercepted by BuildTray to
// hide-to-tray instead of quit (when tray is available).
func (a *App) BuildMainWindow() fyne.Window {
	w := a.fyneApp.NewWindow(i18n.T("ui.window.title"))
	w.Resize(defaultWindowSize)
	w.CenterOnScreen()
	w.SetContent(a.buildTabs())
	a.window = w
	w.Show()
	return w
}

func (a *App) buildTabs() *container.AppTabs {
	return container.NewAppTabs(
		container.NewTabItemWithIcon(i18n.T("ui.tab.status"), theme.HomeIcon(), screens.NewStatusScreen(a.srv)),
		container.NewTabItemWithIcon(i18n.T("ui.tab.settings"), theme.SettingsIcon(), screens.NewSettingsScreen(a)),
		container.NewTabItemWithIcon(i18n.T("ui.tab.logs"), theme.DocumentIcon(), screens.NewLogScreen(a.tap)),
		container.NewTabItemWithIcon(i18n.T("ui.tab.certs"), theme.AccountIcon(), screens.NewCertScreen(a.srv)),
		container.NewTabItemWithIcon(i18n.T("ui.tab.about"), theme.InfoIcon(), screens.NewAboutScreen(config.Version)),
	)
}

// RebuildContent swaps the main window's content with a freshly built
// tab container so translated labels pick up the active locale after a
// language change. No-op when BuildMainWindow has not run yet.
func (a *App) RebuildContent() {
	if a.window == nil {
		return
	}
	a.window.SetContent(a.buildTabs())
}
