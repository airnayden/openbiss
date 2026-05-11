//go:build darwin

package i18n

import (
	"os"
	"os/exec"
	"strings"
)

func detectOSLocale() string {
	out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output()
	if err == nil {
		locale := strings.TrimSpace(string(out))
		locale = strings.Split(locale, "@")[0]
		if len(locale) >= 2 {
			return locale[:2]
		}
	}

	if lang := os.Getenv("LANG"); lang != "" {
		return lang
	}

	return "en"
}
