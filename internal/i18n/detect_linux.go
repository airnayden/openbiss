//go:build linux

package i18n

import "os"

func detectOSLocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return "en"
}
