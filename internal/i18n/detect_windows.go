//go:build windows

package i18n

import (
	"os"
	"os/exec"
	"strings"
)

func detectOSLocale() string {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-Command",
		"(Get-Culture).TwoLetterISOLanguageName").Output()
	if err == nil {
		lang := strings.TrimSpace(string(out))
		if len(lang) >= 2 {
			return lang[:2]
		}
	}

	if lang := os.Getenv("LANG"); lang != "" {
		return lang
	}

	return "en"
}
