// Package instance enforces single-instance execution.
//
// A second process invokes TryAcquire, which detects the existing primary via
// an OS-specific IPC endpoint, sends a raise-window request, and returns
// alreadyRunning=true so the caller can exit cleanly. The primary registers
// its raise-window handler via OnRaiseWindow.
package instance

import "sync"

const raiseByte byte = 'R'

const raiseTimeoutMillis = 500

var (
	mu            sync.Mutex
	raiseWindowFn func()
)

// OnRaiseWindow registers fn to run when a peer instance requests that the
// primary's window be raised. Passing nil clears the handler. The callback
// runs on a background goroutine; GUI code must marshal onto the UI thread.
func OnRaiseWindow(fn func()) {
	mu.Lock()
	raiseWindowFn = fn
	mu.Unlock()
}

func invokeRaiseWindow() {
	mu.Lock()
	fn := raiseWindowFn
	mu.Unlock()
	if fn != nil {
		fn()
	}
}
