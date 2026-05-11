// Package logging provides a custom slog.Handler (Tap) that tees log records
// to stderr, an in-memory ring buffer for the GUI log viewer, and a rotating
// disk log file.
package logging

import "sync"

// RingBuffer is a fixed-capacity, thread-safe circular buffer that drops the
// oldest entry when full.
type RingBuffer[T any] struct {
	mu   sync.Mutex
	buf  []T
	head int
	tail int
	size int
	cap  int
}

// NewRingBuffer allocates a RingBuffer with the given capacity.
func NewRingBuffer[T any](cap int) *RingBuffer[T] {
	return &RingBuffer[T]{
		buf: make([]T, cap),
		cap: cap,
	}
}

// PushDropOldest inserts v into the ring. If the buffer is full the oldest
// entry is overwritten and dropped is returned as true.
func (r *RingBuffer[T]) PushDropOldest(v T) (dropped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buf[r.head%r.cap] = v
	r.head++

	if r.size < r.cap {
		r.size++
		return false
	}
	// Buffer full — advance tail to discard the oldest entry.
	r.tail++
	return true
}

// Snapshot returns a copy of all valid entries in insertion order (oldest
// first). The returned slice is safe for the caller to read without a lock.
func (r *RingBuffer[T]) Snapshot() []T {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]T, r.size)
	for i := 0; i < r.size; i++ {
		result[i] = r.buf[(r.tail+i)%r.cap]
	}
	return result
}
