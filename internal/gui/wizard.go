package gui

import (
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/pkcs11"
)

var wizardSize = fyne.NewSize(480, 320)

const toastDuration = 3 * time.Second

// MaybeShowWizard renders the first-run setup wizard exactly once per
// installation: the second launch finds $DataDir/config.json (written by
// either the Finish or Cancel path) and short-circuits. The wizard is a
// modal three-screen flow over the main window — language selection,
// smart card middleware auto-detection, and a finish message — with
// Cancel/dismiss writing a default config and showing a toast so the
// user always ends up with a config.json on disk.
//
// MUST be called from the Fyne main goroutine (i.e. from inside fyne.Do
// after fyneApp.Run has started).
func (a *App) MaybeShowWizard() {
	jsonPath := filepath.Join(a.cfg.DataDir, "config.json")
	if _, err := os.Stat(jsonPath); err == nil {
		return
	}
	if a.window == nil {
		return
	}
	a.showWizard()
}

func (a *App) showWizard() {
	screen := 0
	selectedLang := a.cfg.Lang
	finished := false
	detectStarted := false

	// Language labels stay in their native script — the user is picking the
	// language, so rendering "Bulgarian" in English locale would defeat
	// the purpose. Matches the literal wording in the task spec.
	const labelEnglish = "English"
	const labelBulgarian = "Български"

	langGroup := widget.NewRadioGroup(
		[]string{labelEnglish, labelBulgarian},
		func(s string) {
			if s == labelBulgarian {
				selectedLang = "bg"
			} else {
				selectedLang = "en"
			}
		},
	)
	if a.cfg.Lang == "bg" {
		langGroup.SetSelected(labelBulgarian)
	} else {
		langGroup.SetSelected(labelEnglish)
	}

	welcomeTitle := func() *widget.Label {
		return widget.NewLabelWithStyle(
			i18n.T("ui.wizard.welcome"),
			fyne.TextAlignCenter,
			fyne.TextStyle{Bold: true},
		)
	}

	screen1 := container.NewVBox(
		welcomeTitle(),
		widget.NewSeparator(),
		widget.NewLabel(i18n.T("ui.wizard.lang.prompt")),
		langGroup,
	)

	detectedLabel := widget.NewLabel(i18n.T("ui.wizard.detect"))
	spinner := widget.NewProgressBarInfinite()

	screen2 := container.NewVBox(
		welcomeTitle(),
		widget.NewSeparator(),
		spinner,
		detectedLabel,
	)

	screen3 := container.NewVBox(
		welcomeTitle(),
		widget.NewSeparator(),
		widget.NewLabel(i18n.T("ui.wizard.finish")),
	)

	screens := []fyne.CanvasObject{screen1, screen2, screen3}
	body := container.NewStack(screens[0])

	var (
		d         *dialog.CustomDialog
		backBtn   *widget.Button
		nextBtn   *widget.Button
		finishBtn *widget.Button
		cancelBtn *widget.Button
		render    func()
	)

	cancelBtn = widget.NewButton("Cancel", func() {
		d.Hide()
	})

	backBtn = widget.NewButton("Back", func() {
		if screen > 0 {
			screen--
			render()
		}
	})

	nextBtn = widget.NewButton("Next", func() {
		if screen < len(screens)-1 {
			screen++
			render()
		}
	})

	finishBtn = widget.NewButton("Finish", func() {
		finished = true
		a.cfg.Lang = selectedLang
		_ = a.cfg.Save()
		i18n.Init(a.cfg.Lang)
		a.RebuildContent()
		a.window.Show()
		d.Hide()
	})
	finishBtn.Importance = widget.HighImportance

	// PKCS#11 work always runs off the UI thread per project convention,
	// even when (as here) DiscoverLibraries only does file-existence
	// checks — future versions may dlopen.
	startDetection := func() {
		if detectStarted {
			return
		}
		detectStarted = true
		go func() {
			libs := pkcs11.DiscoverLibraries("")
			msg := i18n.T("ui.certs.empty")
			if len(libs) > 0 {
				msg = libs[0]
			}
			fyne.Do(func() {
				detectedLabel.SetText(msg)
				spinner.Stop()
				spinner.Hide()
			})
		}()
	}

	render = func() {
		body.Objects = []fyne.CanvasObject{screens[screen]}
		body.Refresh()

		buttons := []fyne.CanvasObject{cancelBtn}
		if screen > 0 {
			buttons = append(buttons, backBtn)
		}
		if screen < len(screens)-1 {
			buttons = append(buttons, nextBtn)
		} else {
			buttons = append(buttons, finishBtn)
		}
		d.SetButtons(buttons)

		if screen == 1 {
			startDetection()
		}
	}

	d = dialog.NewCustomWithoutButtons(i18n.T("ui.wizard.welcome"), body, a.window)
	d.Resize(wizardSize)
	d.SetOnClosed(func() {
		if finished {
			return
		}
		_ = a.cfg.Save()
		a.window.Show()
		a.ShowToast(i18n.T("ui.wizard.cancel.toast"))
	})

	render()
	d.Show()
}

// ShowToast presents a transient popup near the bottom-centre of the main
// window. MUST be called from the Fyne UI thread; the auto-hide uses
// fyne.Do because time.AfterFunc fires on its own goroutine.
func (a *App) ShowToast(msg string) {
	if a.window == nil {
		return
	}
	popup := widget.NewPopUp(widget.NewLabel(msg), a.window.Canvas())
	canvasSize := a.window.Canvas().Size()
	popSize := popup.MinSize()
	pos := fyne.NewPos(
		(canvasSize.Width-popSize.Width)/2,
		canvasSize.Height-popSize.Height-40,
	)
	popup.ShowAtPosition(pos)

	time.AfterFunc(toastDuration, func() {
		fyne.Do(func() {
			popup.Hide()
		})
	})
}
