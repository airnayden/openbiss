package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/airnayden/openbiss/assets"
	"github.com/airnayden/openbiss/internal/i18n"
)

const (
	aboutBundleID = "com.openbiss.openbiss"
	aboutAuthor   = "Nayden Panchev"
	aboutIconSize = 128
)

// NewAboutScreen returns the content widget for the About tab: a fully
// static panel with the application icon, version, bundle identifier,
// and author.
//
// Static by design — no goroutines, no timers, no network calls. Opening
// this tab MUST NOT trigger any outbound traffic (no update probe, no
// telemetry, no version check).
//
// version is the LDFLAGS-injected build tag ("1.0" for releases, "dev"
// for local builds), interpolated into the version label.
func NewAboutScreen(version string) fyne.CanvasObject {
	icon := canvas.NewImageFromResource(fyne.NewStaticResource("icon.png", assets.IconPNG))
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSize(aboutIconSize, aboutIconSize))

	versionLabel := widget.NewLabelWithStyle(
		i18n.T("ui.about.version", version),
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)
	bundleLabel := widget.NewLabelWithStyle(
		i18n.T("ui.about.bundleid", aboutBundleID),
		fyne.TextAlignCenter,
		fyne.TextStyle{},
	)
	authorLabel := widget.NewLabelWithStyle(
		i18n.T("ui.about.author", aboutAuthor),
		fyne.TextAlignCenter,
		fyne.TextStyle{},
	)

	content := container.NewVBox(
		container.NewCenter(icon),
		versionLabel,
		bundleLabel,
		authorLabel,
	)
	return container.NewPadded(content)
}
