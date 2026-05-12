package screens

import (
	"log/slog"
	"strings"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/openbiss/openbiss/internal/i18n"
	"github.com/openbiss/openbiss/internal/logging"
)

// NewLogScreen returns the Logs tab: a live tail of the slog ring buffer
// with an auto-scroll toggle, Clear button, and dropped-count label.
//
// A background goroutine ranges over the Tap's wake channel; on each
// wake it snapshots the ring, formats the records, and pushes the result
// onto the UI thread via fyne.Do. When auto-scroll is enabled the scroll
// container is forced to its bottom on every update.
//
// Clear only clears the displayed text — the underlying ring buffer keeps
// receiving records. A skip counter captured at Clear time hides earlier
// records from subsequent renders without mutating the ring.
//
// Passing a nil tap returns a placeholder label so the screen renders in
// test harnesses where main.go has not yet wired the Tap.
func NewLogScreen(tap *logging.Tap) fyne.CanvasObject {
	if tap == nil {
		return container.NewPadded(widget.NewLabel(i18n.T("ui.logs.empty")))
	}

	wake, snap := tap.Subscribe()

	// TextGrid is Fyne's read-only monospaced display widget. It renders
	// at full contrast (unlike a disabled MultiLineEntry which the theme
	// fades to ~50% opacity) and supports text selection for copying.
	logEntry := widget.NewTextGrid()

	scroll := container.NewScroll(logEntry)

	var autoScroll atomic.Bool
	autoScroll.Store(true)

	autoScrollCheck := widget.NewCheck(i18n.T("ui.logs.autoscroll"), func(b bool) {
		autoScroll.Store(b)
	})
	autoScrollCheck.SetChecked(true)

	var skipCount atomic.Int64

	clearBtn := widget.NewButton(i18n.T("ui.logs.clear"), func() {
		skipCount.Store(int64(len(snap())))
		logEntry.SetText("")
	})

	droppedLabel := widget.NewLabel("")
	droppedLabel.Hide()

	controls := container.NewHBox(autoScrollCheck, clearBtn, droppedLabel)

	render := func() {
		records := snap()
		skip := int(skipCount.Load())
		if skip > len(records) {
			skip = len(records)
		}
		text := formatRecords(records[skip:])

		dropped := tap.DroppedCount()

		fyne.Do(func() {
			logEntry.SetText(text)
			if dropped > 0 {
				droppedLabel.SetText(i18n.T("ui.logs.dropped", dropped))
				droppedLabel.Show()
			} else {
				droppedLabel.Hide()
			}
			if autoScroll.Load() {
				scroll.ScrollToBottom()
			}
		})
	}

	// NewLogScreen is called before fyneApp.Run() starts the event loop.
	// Calling render() (or consuming a buffered wake) here causes fyne.Do to
	// run inline on this goroutine instead of the Fyne main goroutine, which
	// trips the threading check and creates a cascading error storm.
	// Drain the pre-event-loop wake signal (capacity 1) and then block; the
	// for-range only fires after Run() is dispatching closures correctly.
	go func() {
		select {
		case <-wake:
		default:
		}
		for range wake {
			render()
		}
	}()

	return container.NewBorder(controls, nil, nil, nil, scroll)
}

func formatRecords(records []slog.Record) string {
	if len(records) == 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range records {
		b.WriteString(r.Time.Format("15:04:05"))
		b.WriteByte(' ')
		b.WriteString(r.Level.String())
		b.WriteByte(' ')
		b.WriteString(r.Message)
		b.WriteByte('\n')
	}
	return b.String()
}
