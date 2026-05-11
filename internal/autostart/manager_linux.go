//go:build linux

package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const linuxDesktopBaseName = "openbiss.desktop"

type linuxManager struct{}

func newManager() Manager { return linuxManager{} }

func (linuxManager) desktopPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("autostart: resolve home: %w", err)
	}
	return filepath.Join(home, ".config", "autostart", linuxDesktopBaseName), nil
}

func (m linuxManager) Enable(_, appPath string) error {
	path, err := m.desktopPath()
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("autostart: enable: mkdir: %w", err)
	}
	contents := buildLinuxDesktop(appPath)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("autostart: enable: write desktop: %w", err)
	}
	return nil
}

func (m linuxManager) Disable(_ string) error {
	path, err := m.desktopPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("autostart: disable: %w", err)
	}
	return nil
}

func (m linuxManager) IsEnabled(_ string) (bool, error) {
	path, err := m.desktopPath()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("autostart: is_enabled: %w", err)
	}
	return strings.Contains(string(data), "X-GNOME-Autostart-enabled=true"), nil
}

func buildLinuxDesktop(appPath string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=OpenBISS\n" +
		"Exec=" + appPath + "\n" +
		"Hidden=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
}
