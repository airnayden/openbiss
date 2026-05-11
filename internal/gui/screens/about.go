package screens

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/openbiss/openbiss/assets"
	"github.com/openbiss/openbiss/internal/i18n"
)

const (
	aboutGitHubURL = "https://github.com/openbiss/openbiss"
	aboutBundleID  = "com.openbiss.openbiss"
	aboutIconSize  = 128
)

// NewAboutScreen returns the content widget for the About tab: a fully
// static panel with the application icon, version, bundle identifier,
// GitHub link, license, and a "Known behaviors" disclosure section.
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
	licenseLabel := widget.NewLabelWithStyle(
		i18n.T("ui.about.license"),
		fyne.TextAlignCenter,
		fyne.TextStyle{},
	)

	// url.Parse on a compile-time constant URL cannot fail; the
	// discarded error is intentional.
	gitHubURL, _ := url.Parse(aboutGitHubURL)
	gitHubLink := widget.NewHyperlink(i18n.T("ui.about.github"), gitHubURL)
	gitHubLink.Alignment = fyne.TextAlignCenter

	behaviorsHeading := widget.NewLabelWithStyle(
		i18n.T("ui.about.behaviors.title"),
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)

	pinCancelLabel := widget.NewLabel(i18n.T("ui.about.behaviors.pincancel"))
	pinCancelLabel.Wrapping = fyne.TextWrapWord

	unsignedLabel := widget.NewLabel(i18n.T("ui.about.behaviors.unsigned"))
	unsignedLabel.Wrapping = fyne.TextWrapWord

	content := container.NewVBox(
		container.NewCenter(icon),
		versionLabel,
		bundleLabel,
		container.NewCenter(gitHubLink),
		licenseLabel,
		widget.NewSeparator(),
		behaviorsHeading,
		pinCancelLabel,
		unsignedLabel,
	)
	return container.NewPadded(content)
}
