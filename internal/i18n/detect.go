package i18n

import (
	"os"
	"strings"
)

var supportedLangs = map[string]bool{
	"en": true,
	"bg": true,
}

func DetectLanguage() string {
	if override := os.Getenv("OPENBISS_LANG"); override != "" {
		return normalizeLang(override)
	}
	return normalizeLang(detectOSLocale())
}

func normalizeLang(raw string) string {
	if len(raw) < 2 {
		return "en"
	}
	code := strings.ToLower(raw[:2])
	if code == "c" || code == "po" {
		return "en"
	}
	if supportedLangs[code] {
		return code
	}
	return "en"
}
