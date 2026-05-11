package tls

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/openbiss/openbiss/internal/i18n"
)

func trustCert(certPath string) {
	dest := "/usr/local/share/ca-certificates/openbiss-localhost.crt"
	slog.Info(i18n.T("tls.import_linux"))

	input, err := os.ReadFile(certPath)
	if err != nil {
		slog.Warn(i18n.T("tls.import_linux_failed"), "error", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		slog.Warn(i18n.T("tls.import_linux_failed_cmd", certPath, dest), "error", err)
		return
	}

	if err := os.WriteFile(dest, input, 0o644); err != nil {
		slog.Warn(i18n.T("tls.import_linux_failed_cmd", certPath, dest), "error", err)
		return
	}

	cmd := exec.Command("update-ca-certificates")
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Warn(i18n.T("tls.import_linux_update_failed"), "error", err, "output", string(out))
	} else {
		slog.Info(i18n.T("tls.import_linux_success"))
	}
}
