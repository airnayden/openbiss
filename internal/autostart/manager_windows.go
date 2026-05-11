//go:build windows

package autostart

import (
	"fmt"
	"os/exec"
	"strings"
)

const windowsRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

type windowsManager struct{}

func newManager() Manager { return windowsManager{} }

func (m windowsManager) Enable(appName, appPath string) error {
	enabled, err := m.IsEnabled(appName)
	if err != nil {
		return fmt.Errorf("autostart: enable: %w", err)
	}
	if enabled {
		return nil
	}
	out, err := exec.Command("reg.exe", "add", windowsRunKey,
		"/v", appName, "/t", "REG_SZ", "/d", appPath, "/f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("autostart: enable: reg add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m windowsManager) Disable(appName string) error {
	enabled, err := m.IsEnabled(appName)
	if err != nil {
		return fmt.Errorf("autostart: disable: %w", err)
	}
	if !enabled {
		return nil
	}
	out, err := exec.Command("reg.exe", "delete", windowsRunKey,
		"/v", appName, "/f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("autostart: disable: reg delete: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (windowsManager) IsEnabled(appName string) (bool, error) {
	cmd := exec.Command("reg.exe", "query", windowsRunKey, "/v", appName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// reg.exe exits non-zero when the value is missing; treat that as "not enabled"
		// rather than a hard error so IsEnabled stays a pure predicate.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("autostart: is_enabled: reg query: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.Contains(string(out), appName), nil
}
