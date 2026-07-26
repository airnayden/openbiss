package tls

import (
	"log/slog"
	"os/exec"

	"github.com/airnayden/openbiss/internal/i18n"
)

func trustCert(certPath string) {
	slog.Info(i18n.T("tls.import_windows"))
	cmd := exec.Command("certutil", "-addstore", "-user", "Root", certPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Warn(i18n.T("tls.import_windows_failed", certPath),
			"error", err, "output", string(out))
		return
	}
	slog.Info(i18n.T("tls.import_windows_success"))
}
