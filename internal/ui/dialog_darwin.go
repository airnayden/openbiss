//go:build darwin

package ui

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/airnayden/openbiss/internal/i18n"
)

// NewNative returns the OS-native DialogProvider for macOS (osascript-based).
// Call this once at startup and pass the result via constructor injection.
func NewNative() DialogProvider { return &darwinDialog{} }

// darwinDialog implements DialogProvider using osascript (AppleScript) on macOS.
// osascript dialogs are shown in the foreground and integrate with macOS
// accessibility and dark mode automatically.
type darwinDialog struct{}

// PromptPIN displays a secure text input dialog via osascript.
// The 'default answer ""' with 'hidden answer true' suppresses echo in the
// AppleScript text field, providing the same UX as a password field.
// Returns ErrCancelled if the user clicks Cancel or presses Escape.
func (d *darwinDialog) PromptPIN(title, message string) ([]byte, error) {
	script := fmt.Sprintf(`tell application "System Events"
activate
display dialog "%s" with title "%s" default answer "" with hidden answer buttons {"Cancel", "OK"} default button "OK"
text returned of result
end tell`, message, title)

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
			if strings.Contains(stderr, "User canceled") || strings.Contains(stderr, "-128") {
				return nil, ErrCancelled
			}
		}
		fmt.Fprintf(os.Stderr, "osascript PIN dialog failed: %v\nstderr: %s\n", err, stderr)
		return promptPINFromStdin(message)
	}

	return bytes.TrimRight(out, "\n"), nil
}

// SelectCertificate displays a list selection dialog via osascript.
// The choose from list command renders a native macOS list picker.
func (d *darwinDialog) SelectCertificate(title string, options []string) (int, error) {
	// Build the AppleScript list literal: {"item1", "item2", ...}
	quoted := make([]string, len(options))
	for i, opt := range options {
		// Escape double quotes in the option text.
		escaped := strings.ReplaceAll(opt, `"`, `\"`)
		quoted[i] = fmt.Sprintf("%q", escaped)
	}
	listLiteral := "{" + strings.Join(quoted, ", ") + "}"

	script := fmt.Sprintf(`tell application "System Events"
activate
set chosen to choose from list %s with title "%s" with prompt "%s"
if chosen is false then
error "Cancelled" number -128
end if
item 1 of chosen
end tell`, listLiteral, title, i18n.T("dialog.cert_prompt"))

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return 0, fmt.Errorf("%s", i18n.T("error.cert_cancelled"))
		}
		return selectFromStdin(options)
	}

	chosen := strings.TrimRight(string(out), "\n")
	for i, opt := range options {
		if opt == chosen {
			return i, nil
		}
	}

	// If the exact match fails (unlikely), fall back to stdin selection.
	return selectFromStdin(options)
}

// promptPINFromStdin is the TTY fallback for headless environments.
// Note: on macOS the terminal will echo characters unless the caller
// configures raw mode. For headless/CI use this is acceptable.
func promptPINFromStdin(message string) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "%s: ", message)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return []byte(scanner.Text()), nil
	}
	return nil, fmt.Errorf("%s", i18n.T("error.read_pin_stdin"))
}

func selectFromStdin(options []string) (int, error) {
	fmt.Fprintln(os.Stderr, i18n.T("dialog.select_cert"))
	for i, opt := range options {
		fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, opt)
	}
	fmt.Fprint(os.Stderr, i18n.T("dialog.enter_number"))

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return 0, fmt.Errorf("%s", i18n.T("error.read_selection_stdin"))
	}

	n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || n < 1 || n > len(options) {
		return 0, fmt.Errorf("%s", i18n.T("error.invalid_selection"))
	}

	return n - 1, nil
}
