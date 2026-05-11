//go:build linux

package gui

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func hasStatusNotifierWatcher() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"dbus-send",
		"--session",
		"--print-reply",
		"--dest=org.freedesktop.DBus",
		"/org/freedesktop/DBus",
		"org.freedesktop.DBus.NameHasOwner",
		"string:org.kde.StatusNotifierWatcher",
	)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "boolean true")
}
