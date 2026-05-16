package screens

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/server"
)

const apiPollInterval = 500 * time.Millisecond

// NewAPIScreen returns the API tab content: per-endpoint counters,
// success-rate summary, a clickable list of recent requests, and a
// detail panel that renders the full request/response envelope (method,
// path, status, duration, headers, bodies) for the selected entry.
//
// Click handling: rows in the recent-requests list are clickable via
// widget.List.OnSelected. Selection is anchored to the chosen record's
// timestamp (not its list position) so the highlight follows the record
// across new arrivals and ring-buffer evictions. If the selected record
// is evicted, the panel falls back to the newest entry.
//
// A 2 Hz polling goroutine reads srv.Stats() and pushes UI updates onto
// the main goroutine via fyne.Do. Change detection (compare against
// previous-tick values) avoids redundant Fyne layout passes.
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

	addressLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	versionLabel := widget.NewLabel("")
	getsignerLabel := widget.NewLabel("")
	signLabel := widget.NewLabel("")
	otherLabel := widget.NewLabel("")
	successRateLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	emptyListLabel := widget.NewLabel(i18n.T("ui.api.empty"))

	// detailView renders the selected request's envelope. TextGrid is
	// the same widget the Logs tab uses — full contrast (no theme
	// fade) and monospaced.
	detailView := widget.NewTextGrid()
	detailView.SetText(i18n.T("ui.api.detail.empty"))
	detailScroll := container.NewScroll(detailView)

	// snapshot holds the chronological records slice (oldest first)
	// that the list closures and refresh loop both read. atomic.Pointer
	// lets the Fyne render goroutine and the 2 Hz poller share data
	// without a mutex.
	var snapshot atomic.Pointer[[]server.RequestRecord]
	empty := []server.RequestRecord{}
	snapshot.Store(&empty)

	// selectedTime is the Time.UnixNano() of the explicitly clicked
	// record, or 0 when the user has not made a selection. Anchoring
	// by timestamp (rather than chronological index) keeps the
	// highlight on the same record across ring shifts.
	var selectedTime atomic.Int64

	// suppressOnSelect short-circuits widget.List.OnSelected when the
	// refresh loop calls list.Select(listID) to sync the highlight
	// with the timestamp-anchored selection — without this guard the
	// programmatic Select would recurse into the click handler.
	var suppressOnSelect atomic.Bool

	list := widget.NewList(
		func() int {
			recs := *snapshot.Load()
			return len(recs)
		},
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.TextStyle = fyne.TextStyle{Monospace: true}
			return l
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			recs := *snapshot.Load()
			chronoIdx := len(recs) - 1 - int(id)
			if chronoIdx < 0 || chronoIdx >= len(recs) {
				return
			}
			obj.(*widget.Label).SetText(formatRecentLine(recs[chronoIdx]))
		},
	)

	var prev struct {
		address, version, getsigner, sign, other, rate, detail string
		dataLen                                                int
		listID                                                 int
	}
	prev.listID = -1
	prev.dataLen = -1

	stats := srv.Stats()

	refresh := func() {
		nextAddress := addressLine(srv)
		nextVersion := endpointLine("GET /version", &stats.Version)
		nextGetSigner := endpointLine("POST /getsigner", &stats.GetSigner)
		nextSign := endpointLine("POST /sign", &stats.Sign)
		nextOther := endpointLine("other", &stats.Other)
		nextRate := successRateLine(stats)

		records := stats.Recent()
		snapshot.Store(&records)

		chronoIdx, newListID := resolveSelection(records, selectedTime.Load())

		var nextDetail string
		if chronoIdx >= 0 {
			nextDetail = formatDetail(records[chronoIdx])
		} else {
			nextDetail = i18n.T("ui.api.detail.empty")
		}

		dataChanged := prev.dataLen != len(records)
		selectionChanged := prev.listID != newListID

		var changes []func()
		if nextAddress != prev.address {
			prev.address = nextAddress
			v := nextAddress
			changes = append(changes, func() { addressLabel.SetText(v) })
		}
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
		if nextDetail != prev.detail {
			prev.detail = nextDetail
			v := nextDetail
			changes = append(changes, func() { detailView.SetText(v) })
		}
		if dataChanged {
			prev.dataLen = len(records)
			showEmpty := len(records) == 0
			changes = append(changes, func() {
				list.Refresh()
				if showEmpty {
					emptyListLabel.Show()
				} else {
					emptyListLabel.Hide()
				}
			})
		}
		if selectionChanged {
			prev.listID = newListID
			id := newListID
			recCount := len(records)
			changes = append(changes, func() {
				if id >= 0 && id < recCount {
					suppressOnSelect.Store(true)
					list.Select(id)
					suppressOnSelect.Store(false)
				} else {
					list.UnselectAll()
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

	list.OnSelected = func(id widget.ListItemID) {
		if suppressOnSelect.Load() {
			return
		}
		recs := *snapshot.Load()
		chronoIdx := len(recs) - 1 - int(id)
		if chronoIdx < 0 || chronoIdx >= len(recs) {
			return
		}
		selectedTime.Store(recs[chronoIdx].Time.UnixNano())
		go refresh()
	}

	refreshBtn := widget.NewButton(i18n.T("ui.api.refresh"), func() { go refresh() })

	// openDocsBtn launches the embedded Swagger UI in the user's default
	// browser. srv.Port() returns the actually-bound port, so the URL
	// follows the auto-selected port rather than being hardcoded. If the
	// server isn't running (port == 0), the click is a defensive no-op.
	openDocsBtn := widget.NewButton(i18n.T("ui.api.open_docs"), func() {
		port := srv.Port()
		if port == 0 {
			return
		}
		u, err := url.Parse(fmt.Sprintf("https://127.0.0.1:%d/docs/", port))
		if err != nil {
			return
		}
		_ = fyne.CurrentApp().OpenURL(u)
	})

	clearSelectionBtn := widget.NewButton(i18n.T("ui.api.clear_selection"), func() {
		selectedTime.Store(0)
		go refresh()
	})

	go func() {
		t := time.NewTicker(apiPollInterval)
		defer t.Stop()
		for range t.C {
			refresh()
		}
	}()

	header := container.NewVBox(
		addressLabel,
		widget.NewSeparator(),
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
			clearSelectionBtn,
			openDocsBtn,
		),
		widget.NewLabel(i18n.T("ui.api.click_hint")),
	)

	listWithOverlay := container.NewStack(list, container.NewCenter(emptyListLabel))

	body := container.NewVSplit(listWithOverlay, detailScroll)
	body.SetOffset(0.4)

	return container.NewBorder(header, nil, nil, nil, body)
}

// addressLine renders "Address: https://127.0.0.1:53952" when the
// server is running, falling back to the localised lifecycle state
// (Stopped / Starting… / Stopping…) otherwise. Re-evaluated each refresh
// tick so a stop-start cycle that binds a different port from the BISS
// pool (53952–53955) propagates to the UI automatically.
func addressLine(srv *server.Server) string {
	if srv.State() == server.StateRunning {
		if port := srv.Port(); port != 0 {
			return i18n.T("ui.api.address", fmt.Sprintf("https://127.0.0.1:%d", port))
		}
	}
	return i18n.T("ui.api.address", i18n.T(stateKey(srv.State())))
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

// resolveSelection returns the chronological index (oldest = 0) and the
// newest-first list ID for the record whose Time.UnixNano() matches
// selectedTimeNano. When no record matches — either because the user has
// not made a selection (selectedTimeNano == 0) or because the previously
// selected record has been evicted from the ring — the function falls
// back to the newest record. Returns (-1, -1) when records is empty.
func resolveSelection(records []server.RequestRecord, selectedTimeNano int64) (chronoIdx, listID int) {
	chronoIdx = -1
	if selectedTimeNano != 0 {
		for i := range records {
			if records[i].Time.UnixNano() == selectedTimeNano {
				chronoIdx = i
				break
			}
		}
	}
	if chronoIdx == -1 && len(records) > 0 {
		chronoIdx = len(records) - 1
	}
	if chronoIdx < 0 {
		return -1, -1
	}
	return chronoIdx, len(records) - 1 - chronoIdx
}

// formatRecentLine renders one row of the recent-requests list as
// "15:04:05 GET /version 200 5ms". The list widget invokes this once
// per visible row.
func formatRecentLine(r server.RequestRecord) string {
	return fmt.Sprintf("%s %s %s %d %dms",
		r.Time.Format("15:04:05"),
		r.Method,
		r.Path,
		r.StatusCode,
		r.DurationMillis,
	)
}

// formatDetail produces the multi-section detail block (summary, request
// headers, request body, response headers, response body) for one
// captured request. Each section is preceded by an i18n header so users
// can scan the panel without knowing English. Empty bodies render as
// "(empty)"; truncated bodies show the dropped-byte count.
func formatDetail(r server.RequestRecord) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "%s %s\n", r.Method, r.Path)
	fmt.Fprintf(&sb, "%s %s\n", i18n.T("ui.api.detail.time"), r.Time.Format("2006-01-02 15:04:05.000"))
	fmt.Fprintf(&sb, "%s %d\n", i18n.T("ui.api.detail.status"), r.StatusCode)
	fmt.Fprintf(&sb, "%s %dms\n", i18n.T("ui.api.detail.duration"), r.DurationMillis)

	sb.WriteString("\n")
	sb.WriteString(i18n.T("ui.api.detail.req_headers"))
	sb.WriteByte('\n')
	writeHeaders(&sb, r.RequestHeaders)

	sb.WriteString("\n")
	sb.WriteString(i18n.T("ui.api.detail.req_body"))
	sb.WriteByte('\n')
	writeBody(&sb, r.RequestBody, r.RequestTrunc)

	sb.WriteString("\n")
	sb.WriteString(i18n.T("ui.api.detail.resp_headers"))
	sb.WriteByte('\n')
	writeHeaders(&sb, r.ResponseHeaders)

	sb.WriteString("\n")
	sb.WriteString(i18n.T("ui.api.detail.resp_body"))
	sb.WriteByte('\n')
	writeBody(&sb, r.ResponseBody, r.ResponseTrunc)

	return sb.String()
}

func writeHeaders(sb *strings.Builder, h http.Header) {
	if len(h) == 0 {
		sb.WriteString("  ")
		sb.WriteString(i18n.T("ui.api.detail.empty_section"))
		sb.WriteByte('\n')
		return
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range h[k] {
			fmt.Fprintf(sb, "  %s: %s\n", k, v)
		}
	}
}

func writeBody(sb *strings.Builder, body string, trunc int) {
	if body == "" {
		sb.WriteString("  ")
		sb.WriteString(i18n.T("ui.api.detail.empty_section"))
		sb.WriteByte('\n')
		if trunc > 0 {
			fmt.Fprintf(sb, "  %s\n", i18n.T("ui.api.detail.truncated", trunc))
		}
		return
	}
	for _, line := range strings.Split(body, "\n") {
		sb.WriteString("  ")
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	if trunc > 0 {
		fmt.Fprintf(sb, "  %s\n", i18n.T("ui.api.detail.truncated", trunc))
	}
}
