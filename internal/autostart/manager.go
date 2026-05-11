// Package autostart manages OS-specific app autostart-on-login entries
// (macOS LaunchAgent, Windows Run registry key, Linux XDG .desktop).
package autostart

// Manager controls whether an application starts automatically on user login.
// All implementations are idempotent.
type Manager interface {
	// Enable registers appPath to run at login. No-op if already enabled.
	Enable(appName, appPath string) error
	// Disable removes the autostart entry. No-op if not enabled.
	Disable(appName string) error
	// IsEnabled reports whether the autostart entry exists.
	IsEnabled(appName string) (bool, error)
}

// New returns the Manager implementation for the current OS.
func New() Manager {
	return newManager()
}
