package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"github.com/airnayden/openbiss/internal/config"
	"github.com/airnayden/openbiss/internal/gui/screens"
	"github.com/airnayden/openbiss/internal/i18n"
)

var defaultWindowSize = fyne.NewSize(800, 600)

// BuildMainWindow constructs the main settings/status window with the
// six-tab layout (Status, API, Settings, Logs, Certificates, About),
// stores it on the App, shows it, and wires the close button to quit
// the app.
func (a *App) BuildMainWindow() fyne.Window {
	w := a.fyneApp.NewWindow(i18n.T("ui.window.title"))
	w.Resize(defaultWindowSize)
	w.CenterOnScreen()
	w.SetContent(a.buildTabs())
	a.window = w
	w.Show()
	w.SetCloseIntercept(func() { a.fyneApp.Quit() })
	return w
}

func (a *App) buildTabs() *container.AppTabs {
	return container.NewAppTabs(
		container.NewTabItemWithIcon(i18n.T("ui.tab.status"), theme.HomeIcon(), screens.NewStatusScreen(a)),
		container.NewTabItemWithIcon(i18n.T("ui.tab.api"), theme.MailComposeIcon(), screens.NewAPIScreen(a.srv)),
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
