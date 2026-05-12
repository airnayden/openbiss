package screens

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/server"
)

const apiPollInterval = 500 * time.Millisecond

// NewAPIScreen returns the API tab content: per-endpoint counters, a
// success-rate summary, and a scrollable list of the most recent
// requests.
//
// A 2 Hz polling goroutine reads srv.Stats() and pushes UI updates onto
// the main goroutine via fyne.Do. Change detection (compare against
// previous-tick strings) avoids redundant Fyne layout passes.
//
// The first refresh waits one tick (apiPollInterval) before firing so
// the Fyne event loop has time to start — preventing the same race
// pattern that bit logs.go in the original main plan.
//
// A nil srv yields a placeholder so the screen still renders in tests.
func NewAPIScreen(srv *server.Server) fyne.CanvasObject {
	if srv == nil {
		return container.NewPadded(widget.NewLabel(i18n.T("ui.api.empty")))
	}

	versionLabel := widget.NewLabel("")
	getsignerLabel := widget.NewLabel("")
	signLabel := widget.NewLabel("")
	otherLabel := widget.NewLabel("")
	successRateLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	recentEntry := widget.NewMultiLineEntry()
	recentEntry.TextStyle = fyne.TextStyle{Monospace: true}
	recentEntry.Wrapping = fyne.TextWrapOff
	recentEntry.Disable()
	recentScroll := container.NewScroll(recentEntry)

	var prev struct {
		version, getsigner, sign, other, rate, recent string
	}

	stats := srv.Stats()

	refresh := func() {
		nextVersion := endpointLine("GET /version", &stats.Version)
		nextGetSigner := endpointLine("POST /getsigner", &stats.GetSigner)
		nextSign := endpointLine("POST /sign", &stats.Sign)
		nextOther := endpointLine("other", &stats.Other)
		nextRate := successRateLine(stats)
		nextRecent := formatRecent(stats.Recent())

		var changes []func()
		if nextVersion != prev.version {
			prev.version = nextVersion
			v := nextVersion
			changes = append(changes, func() { versionLabel.SetText(v) })
		}
		if nextGetSigner != prev.getsigner {
			prev.getsigner = nextGetSigner
			v := nextGetSigner
			changes = append(changes, func() { getsignerLabel.SetText(v) })
		}
		if nextSign != prev.sign {
			prev.sign = nextSign
			v := nextSign
			changes = append(changes, func() { signLabel.SetText(v) })
		}
		if nextOther != prev.other {
			prev.other = nextOther
			v := nextOther
			changes = append(changes, func() { otherLabel.SetText(v) })
		}
		if nextRate != prev.rate {
			prev.rate = nextRate
			v := nextRate
			changes = append(changes, func() { successRateLabel.SetText(v) })
		}
		if nextRecent != prev.recent {
			prev.recent = nextRecent
			v := nextRecent
			changes = append(changes, func() { recentEntry.SetText(v) })
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

	refreshBtn := widget.NewButton(i18n.T("ui.api.refresh"), refresh)

	// 2 Hz poller. First tick after apiPollInterval gives the event loop
	// time to start before any fyne.Do, avoiding the inline-run race that
	// logs.go originally suffered from.
	go func() {
		t := time.NewTicker(apiPollInterval)
		defer t.Stop()
		for range t.C {
			refresh()
		}
	}()

	header := container.NewVBox(
		versionLabel,
		getsignerLabel,
		signLabel,
		otherLabel,
		widget.NewSeparator(),
		successRateLabel,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabelWithStyle(i18n.T("ui.api.recent"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			refreshBtn,
		),
	)

	return container.NewBorder(header, nil, nil, nil, recentScroll)
}

// endpointLine formats one counter row as
// "GET /version — Total 12 / Success 12 / Errors 0".
func endpointLine(label string, c *server.EndpointCounters) string {
	total := c.Total.Load()
	success := c.Success.Load()
	clientErr := c.ClientError.Load()
	serverErr := c.ServerError.Load()
	errors := clientErr + serverErr
	return fmt.Sprintf("%s — %s %d / %s %d / %s %d",
		label,
		i18n.T("ui.api.total"), total,
		i18n.T("ui.api.success"), success,
		i18n.T("ui.api.errors"), errors,
	)
}

// successRateLine computes total successes / total requests across all
// endpoints and renders the result via the i18n format string.
func successRateLine(s *server.RequestStats) string {
	total := s.Version.Total.Load() + s.GetSigner.Total.Load() + s.Sign.Total.Load() + s.Other.Total.Load()
	if total == 0 {
		return i18n.T("ui.api.successrate", 100)
	}
	success := s.Version.Success.Load() + s.GetSigner.Success.Load() + s.Sign.Success.Load() + s.Other.Success.Load()
	pct := int(success * 100 / total)
	return i18n.T("ui.api.successrate", pct)
}

// formatRecent renders the ring as newest-first lines:
//
//	"15:04:05 GET /version 200 5ms"
//
// Returns the i18n "no requests" placeholder when the ring is empty.
func formatRecent(records []server.RequestRecord) string {
	if len(records) == 0 {
		return i18n.T("ui.api.empty")
	}
	var sb strings.Builder
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		sb.WriteString(r.Time.Format("15:04:05"))
		sb.WriteByte(' ')
		sb.WriteString(r.Method)
		sb.WriteByte(' ')
		sb.WriteString(r.Path)
		sb.WriteByte(' ')
		fmt.Fprintf(&sb, "%d", r.StatusCode)
		sb.WriteByte(' ')
		fmt.Fprintf(&sb, "%dms", r.DurationMillis)
		sb.WriteByte('\n')
	}
	return sb.String()
}
