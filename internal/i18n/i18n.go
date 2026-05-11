// Package i18n provides internationalisation support for OpenBISS.
// It uses embedded JSON locale files and a global translator initialised once
// at startup. No external dependencies are required — only the Go standard library.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed locales/*.json
var localesFS embed.FS

// Global translator, initialised once at startup.
var (
	global *Translator
	mu     sync.RWMutex
)

// Translator holds translated messages for one language with an English fallback.
type Translator struct {
	messages map[string]string
	fallback map[string]string
	lang     string
}

// Init sets the global translator for the given language code.
// It loads messages for lang and falls back to English for any missing key.
// Safe to call multiple times; subsequent calls replace the global instance.
func Init(lang string) {
	t := &Translator{lang: lang}
	t.fallback = loadMessages("en")
	t.messages = loadMessages(lang)
	mu.Lock()
	global = t
	mu.Unlock()
}

// T translates key using fmt.Sprintf for any additional args.
// Falls back to English, then returns the key itself when no translation exists.
func T(key string, args ...any) string {
	mu.RLock()
	t := global
	mu.RUnlock()

	if t == nil {
		return key
	}

	msg, ok := t.messages[key]
	if !ok {
		msg, ok = t.fallback[key]
	}
	if !ok {
		return key
	}

	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// Lang returns the active language code, or "en" when the translator is uninitialised.
func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	if global == nil {
		return "en"
	}
	return global.lang
}

// loadMessages reads and unmarshals the locale JSON file for lang.
// Returns an empty map when the file does not exist or cannot be parsed.
func loadMessages(lang string) map[string]string {
	data, err := localesFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]string{}
	}
	return m
}
