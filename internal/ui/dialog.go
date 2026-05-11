// Package ui provides cross-platform native dialog boxes for PIN entry and
// certificate selection. All dialogs are implemented by exec-ing OS-native
// tools (osascript, zenity, PowerShell) — no GUI framework dependency.
//
// Platform dispatch is handled by build-tag files:
//   - dialog_darwin.go   — macOS via osascript
//   - dialog_windows.go  — Windows via PowerShell
//   - dialog_linux.go    — Linux via zenity / kdialog
//
// When no GUI is available (headless environments, TTY fallback) each platform
// file falls back to reading from stdin so OpenBISS is usable in CI or SSH
// sessions.
package ui

import "errors"

// ErrCancelled is returned by PromptPIN when the user dismisses the dialog
// without entering a PIN (e.g. clicks Cancel or presses Escape).
// Callers should treat this as an authorisation refusal (HTTP 401), not an
// internal error (HTTP 500).
var ErrCancelled = errors.New("dialog cancelled")

// DialogProvider is the interface that platform-specific implementations satisfy.
type DialogProvider interface {
	// PromptPIN shows a secure input dialog asking the user for their smart card PIN.
	// The returned []byte is the raw PIN; the caller MUST zero it after use with
	//   defer func() { for i := range pin { pin[i] = 0 } }()
	// Returns ErrCancelled if the user dismisses the dialog without entering a PIN.
	PromptPIN(title, message string) ([]byte, error)

	// SelectCertificate shows a list selection dialog allowing the user to pick
	// one certificate from options. Each entry in options is a human-readable
	// label (subject CN + issuer). Returns the zero-based index of the chosen
	// certificate, or an error if the dialog was cancelled.
	SelectCertificate(title string, options []string) (int, error)
}
