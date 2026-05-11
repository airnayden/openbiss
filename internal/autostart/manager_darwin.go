//go:build darwin

package autostart

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const darwinPlistLabel = "com.openbiss.openbiss"

type darwinManager struct{}

func newManager() Manager { return darwinManager{} }

func (darwinManager) plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("autostart: resolve home: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", darwinPlistLabel+".plist"), nil
}

func (m darwinManager) Enable(_, appPath string) error {
	path, err := m.plistPath()
	if err != nil {
		return err
	}

	enabled, err := m.IsEnabled("")
	if err != nil {
		return fmt.Errorf("autostart: enable: %w", err)
	}
	if enabled {
		return nil
	}

	logPath := os.ExpandEnv("$HOME/.openbiss/openbiss.log")
	contents := buildDarwinPlist(appPath, logPath)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("autostart: enable: mkdir: %w", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("autostart: enable: write plist: %w", err)
	}

	if out, err := exec.Command("launchctl", "load", "-w", path).CombinedOutput(); err != nil {
		return fmt.Errorf("autostart: enable: launchctl load: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m darwinManager) Disable(_ string) error {
	path, err := m.plistPath()
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		return nil
	} else if statErr != nil {
		return fmt.Errorf("autostart: disable: stat: %w", statErr)
	}

	if out, err := exec.Command("launchctl", "unload", "-w", path).CombinedOutput(); err != nil {
		// Tolerate "Could not find specified service" — plist may exist but
		// agent not currently loaded; we still proceed to remove the file.
		msg := strings.TrimSpace(string(out))
		if !strings.Contains(msg, "Could not find") && !strings.Contains(msg, "no such file") {
			return fmt.Errorf("autostart: disable: launchctl unload: %w: %s", err, msg)
		}
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("autostart: disable: remove plist: %w", err)
	}
	return nil
}

func (m darwinManager) IsEnabled(_ string) (bool, error) {
	path, err := m.plistPath()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("autostart: is_enabled: stat: %w", err)
	}

	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return false, fmt.Errorf("autostart: is_enabled: launchctl list: %w", err)
	}
	return bytes.Contains(out, []byte(darwinPlistLabel)), nil
}

func buildDarwinPlist(appPath, logPath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + darwinPlistLabel + `</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + appPath + `</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>` + logPath + `</string>
    <key>StandardErrorPath</key>
    <string>` + logPath + `</string>
</dict>
</plist>
`
}
