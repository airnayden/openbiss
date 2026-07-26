// Package screens provides the content widgets for the five tabs of the
// main OpenBISS window (Status, Settings, Logs, Certificates, About).
// Each NewXxxScreen constructor returns a fyne.CanvasObject that can be
// plugged into a tab without further wiring.
//
// Shared constraints across all screens:
//
//  1. Atomic-only reads from server.Server (lock-free at the 1 Hz refresh rate).
//  2. PKCS#11 calls run on a separate goroutine — NEVER on the UI thread
//     or the 1 Hz poller.
//  3. fyne.Do(...) for every widget mutation that originates outside the
//     Fyne main goroutine.
//  4. SetText only when the new value differs from the previous tick —
//     Fyne still triggers a layout pass on no-op SetText calls.
package screens

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/airnayden/openbiss/internal/i18n"
	"github.com/airnayden/openbiss/internal/server"
)

const (
	statusPollInterval    = time.Second
	certCountPollInterval = 30 * time.Second
)

// StatusHost is the dependency contract the Status screen needs from the
// owning App. Defined here (not in internal/gui) to avoid an import cycle.
// *gui.App satisfies it via structural typing.
type StatusHost interface {
	Server() *server.Server
	StartServer() error
	StopServer()
	IsServerRunning() bool
	MainWindow() fyne.Window
}

func NewStatusScreen(host StatusHost) fyne.CanvasObject {
	if host == nil {
		return container.NewPadded(widget.NewLabel(i18n.T("ui.status.stopped")))
	}
	srv := host.Server()
	if srv == nil {
		return container.NewPadded(widget.NewLabel(i18n.T("ui.status.stopped")))
	}

	stateLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	portLabel := widget.NewLabel("")
	driverLabel := widget.NewLabel("")
	certCountLabel := widget.NewLabel("")
	uptimeLabel := widget.NewLabel("")

	startBtn := widget.NewButton(i18n.T("ui.status.start"), func() {
		if err := host.StartServer(); err != nil {
			slog.Error("status: start failed", "error", err)
		}
	})
	startBtn.Importance = widget.HighImportance

	stopBtn := widget.NewButton(i18n.T("ui.status.stop"), func() {
		dialog.ShowConfirm(
			i18n.T("ui.status.stop"),
			i18n.T("ui.status.stop.confirm"),
			func(ok bool) {
				if !ok {
					return
				}
				go host.StopServer()
			},
			host.MainWindow(),
		)
	})
	stopBtn.Importance = widget.DangerImportance

	var certCount atomic.Int32

	var prev struct {
		state, port, driver, certCount, uptime string
	}
	prevStartEnabled := true
	prevStopEnabled := true

	refresh := func() {
		nextState := i18n.T(stateKey(srv.State()))
		nextPort := i18n.T("ui.status.port", srv.Port())
		nextDriver := driverText(srv)
		nextCertCount := i18n.T("ui.status.certcount", int(certCount.Load()))
		nextUptime := i18n.T("ui.status.uptime", formatUptime(srv.Uptime()))

		running := host.IsServerRunning()
		wantStartEnabled := !running
		wantStopEnabled := running

		var changes []func()

		if nextState != prev.state {
			prev.state = nextState
			v := nextState
			changes = append(changes, func() { stateLabel.SetText(v) })
		}
		if nextPort != prev.port {
			prev.port = nextPort
			v := nextPort
			changes = append(changes, func() { portLabel.SetText(v) })
		}
		if nextDriver != prev.driver {
			prev.driver = nextDriver
			v := nextDriver
			changes = append(changes, func() { driverLabel.SetText(v) })
		}
		if nextCertCount != prev.certCount {
			prev.certCount = nextCertCount
			v := nextCertCount
			changes = append(changes, func() { certCountLabel.SetText(v) })
		}
		if nextUptime != prev.uptime {
			prev.uptime = nextUptime
			v := nextUptime
			changes = append(changes, func() { uptimeLabel.SetText(v) })
		}
		if wantStartEnabled != prevStartEnabled {
			prevStartEnabled = wantStartEnabled
			want := wantStartEnabled
			changes = append(changes, func() {
				if want {
					startBtn.Enable()
				} else {
					startBtn.Disable()
				}
			})
		}
		if wantStopEnabled != prevStopEnabled {
			prevStopEnabled = wantStopEnabled
			want := wantStopEnabled
			changes = append(changes, func() {
				if want {
					stopBtn.Enable()
				} else {
					stopBtn.Disable()
				}
			})
		}

		if len(changes) == 0 {
			return
		}

		fyne.Do(func() {
			for _, c := range changes {
				c()
			}
		})
	}

	refresh()

	go func() {
		t := time.NewTicker(statusPollInterval)
		defer t.Stop()
		for range t.C {
			refresh()
		}
	}()

	go func() {
		t := time.NewTicker(certCountPollInterval)
		defer t.Stop()
		sampleCertCount(srv, &certCount)
		for range t.C {
			sampleCertCount(srv, &certCount)
		}
	}()

	buttonRow := container.NewHBox(startBtn, stopBtn)
	rows := container.NewVBox(
		buttonRow,
		widget.NewSeparator(),
		stateLabel,
		widget.NewSeparator(),
		portLabel,
		driverLabel,
		certCountLabel,
		uptimeLabel,
	)
	return container.NewPadded(rows)
}

func driverText(srv *server.Server) string {
	d := srv.Driver()
	if d == nil {
		return i18n.T("ui.status.driver.none")
	}
	return i18n.T("ui.status.driver", d.LibPath())
}

// sampleCertCount reads the cached driver, asks it for the certificate
// list, and stores the result in dst. Both nil-driver and ListCertificates
// errors collapse to 0 — the user sees "Certificates available: 0" rather
// than a stale count from the previous successful read.
//
// Runs on its own goroutine (NOT the 1 Hz poller) because PKCS#11 calls
// can block on smart-card I/O for hundreds of milliseconds.
func sampleCertCount(srv *server.Server, dst *atomic.Int32) {
	d := srv.Driver()
	if d == nil {
		dst.Store(0)
		return
	}
	certs, err := d.ListCertificates()
	if err != nil {
		slog.Debug("status: cert count sample failed", "error", err)
		dst.Store(0)
		return
	}
	dst.Store(int32(len(certs)))
}

func stateKey(s server.ServerState) string {
	switch s {
	case server.StateRunning:
		return "ui.status.running"
	case server.StateStarting:
		return "ui.status.starting"
	case server.StateStopping:
		return "ui.status.stopping"
	default:
		return "ui.status.stopped"
	}
}

// formatUptime renders d as a compact "5h3m12s" / "3m12s" / "12s" string,
// eliding leading zero-valued units so a freshly-started server reads
// "0s" rather than "0h0m0s".
func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 || h > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	parts = append(parts, fmt.Sprintf("%ds", s))
	return strings.Join(parts, "")
}
