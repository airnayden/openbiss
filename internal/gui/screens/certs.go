package screens

import (
	"errors"
	"log/slog"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/pkcs11"
	"github.com/openbiss/openbiss/internal/server"
)

// certListDateFormat is the user-visible expiry-date format on the cert
// list. ISO-style year-month-day is unambiguous across the en/bg locales
// and avoids the en-US M/D/Y vs bg D.M.Y collision.
const certListDateFormat = "2006-01-02"

// certToastDuration is how long the session-lost toast remains visible.
// Matches gui.toastDuration semantically but is duplicated here because
// internal/gui/screens must not import internal/gui (cycle).
const certToastDuration = 4 * time.Second

// certScreen owns the widgets and reactive state for the Certificates
// tab. A single instance is created per main window and updated in-place
// by the Refresh button and the initial async load.
//
// PKCS#11 enumeration is potentially slow (hundreds of milliseconds of
// smart-card I/O) so it ALWAYS runs on a background goroutine; widget
// mutations are funnelled through fyne.Do.
type certScreen struct {
	srv *server.Server

	spinner    *widget.ProgressBarInfinite
	refreshBtn *widget.Button
	retryBtn   *widget.Button

	list      *widget.List
	emptyMsg  *widget.Label
	errorMsg  *widget.Label
	errorBox  *fyne.Container
	bodyStack *fyne.Container

	// certs is read by widget.List update funcs (UI thread) and written
	// from inside fyne.Do callbacks (also UI thread), so no lock is
	// required. All access stays on the Fyne main goroutine.
	certs []*pkcs11.CertWithSlot

	// callbackRegistered remembers we already wired OnSessionLost so a
	// second Refresh click does not stack callbacks. Touched only from
	// the refresh goroutine — at most one concurrent runner because the
	// Refresh button is disabled while loading.
	callbackRegistered bool
}

// NewCertScreen returns the content widget for the Certificates tab: a
// read-only list of certificates currently visible on the inserted smart
// card. Three columns — Subject CN, Issuer CN, expiry date — populated
// from driver.ListCertificates().
//
// The Refresh button triggers a fresh enumeration in a background
// goroutine; the spinner is shown for the duration. An empty state
// covers both "no card inserted" and "card with no certs" (the driver
// surfaces both as sentinel errors). Other errors render inline with a
// retry button.
//
// driver.OnSessionLost is wired so a card-removal mid-session shows a
// transient toast near the bottom of the window.
//
// A nil srv yields a single placeholder so the tab still renders when
// the screen is constructed in test harnesses or before main.go wires
// the server.
func NewCertScreen(srv *server.Server) fyne.CanvasObject {
	if srv == nil {
		return container.NewPadded(widget.NewLabel(i18n.T("ui.certs.empty")))
	}

	s := &certScreen{srv: srv}

	// Column header row. GridLayout(3) gives equal column widths so the
	// list rows below align under the headers.
	cnHeader := widget.NewLabelWithStyle(i18n.T("ui.certs.column.cn"),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	issuerHeader := widget.NewLabelWithStyle(i18n.T("ui.certs.column.issuer"),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	expiresHeader := widget.NewLabelWithStyle(i18n.T("ui.certs.column.expires"),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	columnHeader := container.New(layout.NewGridLayout(3),
		cnHeader, issuerHeader, expiresHeader)

	// Spinner + Refresh button live on a separate row above the column
	// header. Spinner starts hidden so the static initial UI does not
	// show a stray pulsing bar before the first async load kicks in.
	s.spinner = widget.NewProgressBarInfinite()
	s.spinner.Hide()
	s.refreshBtn = widget.NewButton(i18n.T("ui.certs.refresh"), func() { s.refresh() })

	// NewBorder(right=...) anchors the controls to the right edge of the
	// row; the empty centre/left collapses so the spinner sits next to
	// the Refresh button without ballooning to full width.
	buttonsRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(s.spinner, s.refreshBtn))

	top := container.NewVBox(buttonsRow, widget.NewSeparator(), columnHeader)

	// Body: a stack of three mutually-exclusive views. Only one is
	// .Show()n at a time; the others stay .Hide()den. Stack lets each
	// view fill the available area (centred labels look good against
	// the spacious empty cert tab).
	s.list = widget.NewList(
		func() int { return len(s.certs) },
		func() fyne.CanvasObject {
			return container.New(layout.NewGridLayout(3),
				widget.NewLabel(""),
				widget.NewLabel(""),
				widget.NewLabel(""),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(s.certs) {
				return
			}
			row, ok := o.(*fyne.Container)
			if !ok || len(row.Objects) < 3 {
				return
			}
			cert := s.certs[i].Certificate
			row.Objects[0].(*widget.Label).SetText(cert.Subject.CommonName)
			row.Objects[1].(*widget.Label).SetText(cert.Issuer.CommonName)
			row.Objects[2].(*widget.Label).SetText(cert.NotAfter.Format(certListDateFormat))
		},
	)
	s.list.Hide()

	s.emptyMsg = widget.NewLabel(i18n.T("ui.certs.empty"))
	s.emptyMsg.Alignment = fyne.TextAlignCenter
	s.emptyMsg.Hide()

	s.errorMsg = widget.NewLabel("")
	s.errorMsg.Wrapping = fyne.TextWrapWord
	s.retryBtn = widget.NewButton(i18n.T("ui.certs.refresh"), func() { s.refresh() })
	s.errorBox = container.NewVBox(s.errorMsg, container.NewCenter(s.retryBtn))
	s.errorBox.Hide()

	s.bodyStack = container.NewStack(
		s.list,
		container.NewCenter(s.emptyMsg),
		container.NewCenter(s.errorBox),
	)

	// Kick off the initial load. refresh() turns on the spinner and
	// dispatches the goroutine, so the constructor returns immediately
	// without blocking on smart-card I/O.
	s.refresh()

	return container.NewBorder(top, nil, nil, nil, s.bodyStack)
}

// refresh resets the screen to its loading state and dispatches a
// background goroutine to enumerate certificates. The Refresh button is
// disabled while loading so rapid clicks cannot stack overlapping
// goroutines (each of which would otherwise serialise on the driver
// mutex anyway, but the UI thrash would be ugly).
//
// Safe to call from any goroutine; every widget mutation is scheduled
// via fyne.Do.
func (s *certScreen) refresh() {
	fyne.Do(func() {
		s.spinner.Show()
		s.refreshBtn.Disable()
		s.list.Hide()
		s.emptyMsg.Hide()
		s.errorBox.Hide()
	})

	go func() {
		driver := s.srv.Driver()
		if driver == nil {
			fyne.Do(func() {
				s.spinner.Hide()
				s.refreshBtn.Enable()
				s.emptyMsg.Show()
			})
			return
		}

		// Driver may have only just become available; register the
		// session-lost callback on first successful access so card
		// removal mid-session triggers the toast.
		s.registerSessionLost(driver)

		certs, err := driver.ListCertificates()
		fyne.Do(func() {
			s.spinner.Hide()
			s.refreshBtn.Enable()
			if err != nil {
				// ErrNoToken / ErrNoCerts are the expected "nothing to
				// show" outcomes — collapse both into the empty state
				// rather than alarming the user with an error banner.
				if errors.Is(err, pkcs11.ErrNoToken) || errors.Is(err, pkcs11.ErrNoCerts) {
					s.emptyMsg.Show()
					return
				}
				slog.Warn("certs: list failed", "error", err)
				s.errorMsg.SetText(err.Error())
				s.errorBox.Show()
				return
			}
			if len(certs) == 0 {
				s.emptyMsg.Show()
				return
			}
			s.certs = certs
			s.list.Refresh()
			s.list.Show()
		})
	}()
}

// registerSessionLost wires the driver's session-lost callback exactly
// once. Subsequent calls are no-ops so repeated Refresh clicks do not
// stack duplicate handlers (which would each fire a separate toast on
// every reconnect).
func (s *certScreen) registerSessionLost(driver *pkcs11.Driver) {
	if s.callbackRegistered {
		return
	}
	s.callbackRegistered = true
	driver.OnSessionLost(func() {
		fyne.Do(func() {
			s.showSessionLostToast()
		})
	})
}

// showSessionLostToast displays a transient popup near the bottom-centre
// of the window owning the cert screen. The window is located via
// CanvasForObject so this package does not import internal/gui (cycle).
//
// Best-effort: if the screen has not yet been attached to a canvas (the
// tab was never opened, or the window was destroyed), the toast silently
// skips — better than crashing on a nil canvas during a card-removal
// event the user will quickly notice anyway.
func (s *certScreen) showSessionLostToast() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	canvas := app.Driver().CanvasForObject(s.bodyStack)
	if canvas == nil {
		return
	}
	popup := widget.NewPopUp(widget.NewLabel(i18n.T("ui.toast.card.removed")), canvas)
	canvasSize := canvas.Size()
	popSize := popup.MinSize()
	popup.ShowAtPosition(fyne.NewPos(
		(canvasSize.Width-popSize.Width)/2,
		canvasSize.Height-popSize.Height-40,
	))
	time.AfterFunc(certToastDuration, func() {
		fyne.Do(func() {
			popup.Hide()
		})
	})
}
