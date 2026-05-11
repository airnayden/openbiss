// Package uifyne provides a Fyne-based implementation of ui.DialogProvider
// for the OpenBISS GUI build. It is a sibling of internal/ui so the headless
// CLI build (which uses osascript / zenity / PowerShell via internal/ui) can
// avoid linking the Fyne runtime.
//
// The package name is uifyne (not fyne) to avoid colliding with the
// fyne.io/fyne/v2 import alias.
//
// Threading: HTTP handler goroutines call into FyneDialog from outside the
// Fyne UI thread. Each method marshals dialog construction onto the UI
// thread via fyne.Do and blocks on a buffered result channel until the user
// dismisses the dialog. The channel buffer (capacity 1) ensures the UI
// callback never blocks if the caller has abandoned the wait.
package uifyne

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/ui"
)

// FyneDialog implements ui.DialogProvider using modal Fyne windows over the
// application's main window. Safe to share across goroutines: methods
// serialise on the Fyne UI thread.
type FyneDialog struct {
	app    fyne.App
	window fyne.Window
}

// New constructs a FyneDialog parented to window. window must be non-nil at
// the time PromptPIN or SelectCertificate is invoked, otherwise Fyne will
// panic when it tries to resolve the parent canvas.
func New(app fyne.App, window fyne.Window) *FyneDialog {
	return &FyneDialog{app: app, window: window}
}

var _ ui.DialogProvider = (*FyneDialog)(nil)

// PromptPIN shows a modal with the given message above a password-masked
// entry. Returns (nil, ui.ErrCancelled) if the user dismisses the dialog.
// The returned []byte is owned by the caller, who must zero it after use
// per the ui.DialogProvider contract.
func (d *FyneDialog) PromptPIN(title, message string) ([]byte, error) {
	type pinResult struct {
		pin []byte
		err error
	}
	ch := make(chan pinResult, 1)

	fyne.Do(func() {
		entry := widget.NewPasswordEntry()
		body := container.NewVBox(
			widget.NewLabel(message),
			widget.NewLabel(i18n.T("ui.dialog.pin.label")),
			entry,
		)

		dialog.ShowCustomConfirm(title, "OK", "Cancel", body, func(confirmed bool) {
			if !confirmed {
				ch <- pinResult{pin: nil, err: ui.ErrCancelled}
				return
			}
			// Copy out of Fyne's internal string buffer, then immediately
			// blank the entry so the PIN doesn't linger as a referenced
			// string in widget state. The string -> []byte copy here is
			// unavoidable: Fyne's Entry stores text as a Go string, so
			// the secret already lives in immutable memory by the time
			// we read it.
			pin := []byte(entry.Text)
			entry.SetText("")
			ch <- pinResult{pin: pin, err: nil}
		}, d.window)
	})

	r := <-ch
	return r.pin, r.err
}

// SelectCertificate shows a modal radio-group picker over the given options
// and returns the zero-based index of the user's choice. Returns
// (-1, ui.ErrCancelled) on cancel or when the user confirms without
// selecting an entry.
func (d *FyneDialog) SelectCertificate(title string, options []string) (int, error) {
	type selResult struct {
		idx int
		err error
	}
	ch := make(chan selResult, 1)

	fyne.Do(func() {
		selected := -1
		group := widget.NewRadioGroup(options, func(choice string) {
			for i, opt := range options {
				if opt == choice {
					selected = i
					return
				}
			}
			selected = -1
		})
		if len(options) > 0 {
			group.SetSelected(options[0])
		}

		body := container.NewVBox(
			widget.NewLabel(i18n.T("dialog.cert_prompt")),
			group,
		)

		dialog.ShowCustomConfirm(title, "OK", "Cancel", body, func(confirmed bool) {
			if !confirmed || selected < 0 {
				ch <- selResult{idx: -1, err: ui.ErrCancelled}
				return
			}
			ch <- selResult{idx: selected, err: nil}
		}, d.window)
	})

	r := <-ch
	return r.idx, r.err
}
