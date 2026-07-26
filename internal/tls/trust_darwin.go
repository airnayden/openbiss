package tls

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/airnayden/openbiss/internal/i18n"
)

func trustCert(certPath string) {
	slog.Info(i18n.T("tls.import_macos"))
	fmt.Println("\n  " + i18n.T("tls.import_macos_prompt"))

	cmd := exec.Command("sudo", "security", "add-trusted-cert", "-d", "-p", "ssl",
		"-k", "/Library/Keychains/System.keychain", certPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		slog.Warn(i18n.T("tls.import_macos_failed"),
			"command", "sudo security add-trusted-cert -d -p ssl -k /Library/Keychains/System.keychain "+certPath,
			"error", err)
		return
	}
	slog.Info(i18n.T("tls.import_macos_success"))
}
