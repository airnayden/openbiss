package screens

import (
	"log/slog"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/openbiss/openbiss/internal/autostart"
	"github.com/openbiss/openbiss/internal/config"
	"github.com/openbiss/openbiss/internal/i18n"
)

// autostartAppName is the canonical name passed to autostart.Manager so
// the registry/plist/.desktop entry matches scripts/install.sh exactly.
// Sharing the literal here means the in-app toggle and the installer
// script touch the same OS resource and stay idempotent across each other.
const autostartAppName = "OpenBISS"

// regenTLSToast is the post-confirmation message after the user requests a
// TLS certificate regeneration. Hard-coded English string: T7 i18n shipped
// with only the dialog title (ui.settings.regen_tls) and confirmation body
// (ui.settings.regen_tls.confirm); no localized key exists for this
// follow-up toast, and the task spec mandates the exact wording.
const regenTLSToast = "New TLS certificate will be generated on next server start. Restart the app to apply."

// SettingsHost is the dependency contract this screen needs from its
// owning Fyne app. *gui.App satisfies it; defining the interface here
// (rather than taking *gui.App directly) breaks the import cycle that
// would otherwise form because internal/gui already imports
// internal/gui/screens for the tab wiring in window.go.
type SettingsHost interface {
	Cfg() *config.Config
	MainWindow() fyne.Window
	RebuildContent()
	ShowToast(msg string)
}

// NewSettingsScreen returns the content widget for the Settings tab with
// six controls in a widget.Form: language (live-applied with toast +
// tab rebuild), log level (saved-only, restart required), PKCS#11 library
// path with Browse, data directory (read-only display), autostart toggle
// (filesystem/registry I/O off the UI thread), and a Regenerate TLS
// Certificate button that asks the user to restart the app.
//
// Every mutation calls cfg.Save() so changes survive a hard kill. The
// PKCS#11 driver and TLS subsystem are NOT hot-reloaded — the user must
// restart for path changes to take effect.
func NewSettingsScreen(host SettingsHost) fyne.CanvasObject {
	cfg := host.Cfg()

	langSelect := buildLangSelect(host, cfg)
	logSelect := buildLogLevelSelect(cfg)
	pkcs11Row := buildPKCS11Row(host, cfg)
	dataDirLabel := buildDataDirLabel(cfg)
	autostartCheck := buildAutostartCheck(host)
	regenBtn := buildRegenTLSButton(host, cfg)

	form := widget.NewForm(
		widget.NewFormItem(i18n.T("ui.settings.lang"), langSelect),
		widget.NewFormItem(i18n.T("ui.settings.loglevel"), logSelect),
		widget.NewFormItem(i18n.T("ui.settings.pkcs11"), pkcs11Row),
		widget.NewFormItem(i18n.T("ui.settings.datadir"), dataDirLabel),
		widget.NewFormItem(i18n.T("ui.settings.autostart"), autostartCheck),
		widget.NewFormItem("", regenBtn),
	)

	return container.NewPadded(form)
}

func buildLangSelect(host SettingsHost, cfg *config.Config) *widget.Select {
	enLabel := i18n.T("ui.settings.lang.en")
	bgLabel := i18n.T("ui.settings.lang.bg")
	codeFor := map[string]string{enLabel: "en", bgLabel: "bg"}
	labelFor := map[string]string{"en": enLabel, "bg": bgLabel}

	s := widget.NewSelect([]string{enLabel, bgLabel}, nil)
	if lbl, ok := labelFor[cfg.Lang]; ok {
		s.SetSelected(lbl)
	} else {
		s.SetSelected(enLabel)
	}
	s.OnChanged = func(label string) {
		newLang, ok := codeFor[label]
		if !ok || newLang == cfg.Lang {
			return
		}
		cfg.Lang = newLang
		if err := cfg.Save(); err != nil {
			slog.Warn("settings: save lang failed", "error", err)
		}
		i18n.Init(newLang)
		host.RebuildContent()
		host.ShowToast(i18n.T("ui.toast.lang.changed"))
	}
	return s
}

func buildLogLevelSelect(cfg *config.Config) *widget.Select {
	levels := []string{"debug", "info", "warn", "error"}
	s := widget.NewSelect(levels, nil)
	s.SetSelected(normalizeLogLevel(cfg.LogLevel))
	s.OnChanged = func(level string) {
		if level == cfg.LogLevel {
			return
		}
		cfg.LogLevel = level
		if err := cfg.Save(); err != nil {
			slog.Warn("settings: save log level failed", "error", err)
		}
	}
	return s
}

func buildPKCS11Row(host SettingsHost, cfg *config.Config) fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(i18n.T("ui.settings.pkcs11.auto"))
	entry.SetText(cfg.PKCS11Lib)
	entry.OnChanged = func(v string) {
		if v == cfg.PKCS11Lib {
			return
		}
		cfg.PKCS11Lib = v
		if err := cfg.Save(); err != nil {
			slog.Warn("settings: save pkcs11 path failed", "error", err)
		}
	}

	browse := widget.NewButton(i18n.T("ui.settings.pkcs11.browse"), func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()
			path := reader.URI().Path()
			entry.SetText(path)
		}, host.MainWindow())
	})

	return container.NewBorder(nil, nil, nil, browse, entry)
}

func buildDataDirLabel(cfg *config.Config) *widget.Label {
	l := widget.NewLabel(cfg.DataDir)
	l.Wrapping = fyne.TextWrapBreak
	return l
}

func buildAutostartCheck(host SettingsHost) *widget.Check {
	mgr := autostart.New()
	check := widget.NewCheck("", nil)

	go func() {
		enabled, err := mgr.IsEnabled(autostartAppName)
		if err != nil {
			slog.Warn("settings: autostart IsEnabled failed", "error", err)
			return
		}
		fyne.Do(func() { check.SetChecked(enabled) })
	}()

	check.OnChanged = func(checked bool) {
		go func() {
			appPath, err := os.Executable()
			if err != nil {
				slog.Warn("settings: os.Executable failed", "error", err)
				fyne.Do(func() {
					check.SetChecked(!checked)
					host.ShowToast(err.Error())
				})
				return
			}
			var opErr error
			if checked {
				opErr = mgr.Enable(autostartAppName, appPath)
			} else {
				opErr = mgr.Disable(autostartAppName)
			}
			if opErr != nil {
				slog.Warn("settings: autostart toggle failed", "checked", checked, "error", opErr)
				fyne.Do(func() {
					check.SetChecked(!checked)
					host.ShowToast(opErr.Error())
				})
			}
		}()
	}
	return check
}

func buildRegenTLSButton(host SettingsHost, cfg *config.Config) *widget.Button {
	btn := widget.NewButton(i18n.T("ui.settings.regen_tls"), func() {
		dialog.ShowConfirm(
			i18n.T("ui.settings.regen_tls"),
			i18n.T("ui.settings.regen_tls.confirm"),
			func(confirmed bool) {
				if !confirmed {
					return
				}
				if err := os.Remove(cfg.TLSCertPath()); err != nil && !os.IsNotExist(err) {
					slog.Warn("settings: remove TLS cert failed", "path", cfg.TLSCertPath(), "error", err)
				}
				if err := os.Remove(cfg.TLSKeyPath()); err != nil && !os.IsNotExist(err) {
					slog.Warn("settings: remove TLS key failed", "path", cfg.TLSKeyPath(), "error", err)
				}
				host.ShowToast(regenTLSToast)
			},
			host.MainWindow(),
		)
	})
	btn.Importance = widget.WarningImportance
	return btn
}

func normalizeLogLevel(level string) string {
	switch level {
	case "debug", "info", "warn", "error":
		return level
	default:
		return "info"
	}
}
