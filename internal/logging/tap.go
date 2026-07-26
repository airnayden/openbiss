package logging

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/airnayden/openbiss/internal/config"
)

const (
	ringCap      = 1000
	maxDiskBytes = 10 * 1024 * 1024
)

// ParseLevel converts a config log level string to slog.Level, defaulting to
// slog.LevelInfo for unrecognised values.
func ParseLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}

// Tap is a slog.Handler that simultaneously writes to:
//   - an inner handler (stderr via TextHandler)
//   - an in-memory ring buffer for the GUI log viewer
//   - a disk file ($DataDir/openbiss.log) with 10 MB truncate-on-rotate
type Tap struct {
	inner     slog.Handler
	ring      *RingBuffer[slog.Record]
	wake      chan struct{}
	dropped   atomic.Int64
	diskMu    sync.Mutex
	diskFile  *os.File
	diskPath  string
	diskBytes int64
}

// NewTap constructs a Tap that writes to stderr at the given level and also
// fans out to the ring buffer and disk log.
func NewTap(cfg *config.Config, level slog.Level) (*Tap, error) {
	diskPath := filepath.Join(cfg.DataDir, "openbiss.log")
	f, err := os.OpenFile(diskPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("logging: open disk log: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("logging: stat disk log: %w", err)
	}

	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})

	return &Tap{
		inner:     inner,
		ring:      NewRingBuffer[slog.Record](ringCap),
		wake:      make(chan struct{}, 1),
		diskFile:  f,
		diskPath:  diskPath,
		diskBytes: info.Size(),
	}, nil
}

// Enabled implements slog.Handler.
func (t *Tap) Enabled(ctx context.Context, level slog.Level) bool {
	return t.inner.Enabled(ctx, level)
}

// Handle implements slog.Handler.
func (t *Tap) Handle(ctx context.Context, r slog.Record) error {
	if dropped := t.ring.PushDropOldest(r); dropped {
		t.dropped.Add(1)
	}

	select {
	case t.wake <- struct{}{}:
	default:
	}

	_ = t.inner.Handle(ctx, r)

	t.writeDisk(r)

	return nil
}

// WithAttrs implements slog.Handler.
func (t *Tap) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Tap{
		inner:     t.inner.WithAttrs(attrs),
		ring:      t.ring,
		wake:      t.wake,
		diskFile:  t.diskFile,
		diskPath:  t.diskPath,
		diskBytes: t.diskBytes,
	}
}

// WithGroup implements slog.Handler.
func (t *Tap) WithGroup(name string) slog.Handler {
	return &Tap{
		inner:     t.inner.WithGroup(name),
		ring:      t.ring,
		wake:      t.wake,
		diskFile:  t.diskFile,
		diskPath:  t.diskPath,
		diskBytes: t.diskBytes,
	}
}

// Subscribe returns the lossy wake channel and a snapshot function. Callers
// receive a signal on the channel whenever new records arrive and call the
// snapshot function to retrieve the current ring contents.
func (t *Tap) Subscribe() (<-chan struct{}, func() []slog.Record) {
	return t.wake, func() []slog.Record { return t.ring.Snapshot() }
}

// DroppedCount returns the total number of records dropped from the ring due
// to overflow.
func (t *Tap) DroppedCount() int64 {
	return t.dropped.Load()
}

func (t *Tap) writeDisk(r slog.Record) {
	var buf bytes.Buffer
	tmp := slog.NewTextHandler(&buf, nil)
	_ = tmp.Handle(context.Background(), r)
	line := buf.Bytes()

	t.diskMu.Lock()
	defer t.diskMu.Unlock()

	n, _ := t.diskFile.Write(line)
	t.diskBytes += int64(n)

	if t.diskBytes >= maxDiskBytes {
		t.rotateLocked()
	}
}

func (t *Tap) rotateLocked() {
	t.diskFile.Close()
	_ = os.Rename(t.diskPath, t.diskPath+".previous")
	f, err := os.OpenFile(t.diskPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	t.diskFile = f
	t.diskBytes = 0
}
