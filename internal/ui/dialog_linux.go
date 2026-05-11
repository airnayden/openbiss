//go:build linux

package ui

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/openbiss/openbiss/internal/i18n"
)

// NewNative returns the OS-native DialogProvider for Linux (zenity/kdialog-based).
// Call this once at startup and pass the result via constructor injection.
func NewNative() DialogProvider { return &linuxDialog{} }

// linuxDialog implements DialogProvider using zenity (GTK) or kdialog (KDE).
// It falls back to stdin for headless / server environments.
type linuxDialog struct{}

// PromptPIN prompts for a PIN using zenity --password, kdialog --password,
// or stdin fallback in that order.
func (d *linuxDialog) PromptPIN(title, message string) ([]byte, error) {
	// zenity (GNOME/GTK environments).
	if path, err := exec.LookPath("zenity"); err == nil {
		out, err := exec.Command(path,
			"--password",
			"--title="+title,
			"--text="+message,
		).Output()
		if err == nil {
			return bytes.TrimRight(out, "\n"), nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, ErrCancelled
		}
	}

	if path, err := exec.LookPath("kdialog"); err == nil {
		out, err := exec.Command(path,
			"--title", title,
			"--password", message,
		).Output()
		if err == nil {
			return bytes.TrimRight(out, "\n"), nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, ErrCancelled
		}
	}

	return promptPINFromStdin(message)
}

// SelectCertificate presents a list picker using zenity or kdialog.
func (d *linuxDialog) SelectCertificate(title string, options []string) (int, error) {
	if path, err := exec.LookPath("zenity"); err == nil {
		args := []string{
			"--list",
			"--title=" + title,
			"--text=" + i18n.T("dialog.cert_prompt"),
			"--column=Certificate",
		}
		args = append(args, options...)

		out, err := exec.Command(path, args...).Output()
		if err == nil {
			chosen := strings.TrimRight(string(out), "\n")
			for i, opt := range options {
				if opt == chosen {
					return i, nil
				}
			}
		} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return 0, fmt.Errorf("%s", i18n.T("error.cert_cancelled"))
		}
	}

	if path, err := exec.LookPath("kdialog"); err == nil {
		args := []string{"--title", title, "--menu", i18n.T("dialog.cert_prompt")}
		for i, opt := range options {
			args = append(args, strconv.Itoa(i), opt)
		}
		out, err := exec.Command(path, args...).Output()
		if err == nil {
			n, err := strconv.Atoi(strings.TrimRight(string(out), "\n"))
			if err == nil && n >= 0 && n < len(options) {
				return n, nil
			}
		} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return 0, fmt.Errorf("%s", i18n.T("error.cert_cancelled"))
		}
	}

	return selectFromStdin(options)
}

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
