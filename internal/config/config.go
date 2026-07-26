// Package config provides runtime configuration for OpenBISS.
// Configuration is resolved with layered precedence: built-in defaults, then
// $DataDir/config.json (if present), then environment variables (which always
// win). Missing config files are tolerated silently; corrupt config files are
// renamed to config.json.broken-<timestamp> and the loader falls back to
// defaults.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/airnayden/openbiss/internal/i18n"
)

// Version is the OpenBISS version string, mirrored in the /version endpoint
// response to maintain BISS API compatibility.
const Version = "1.0"

// BISSPorts are the ports BISS listens on, tried in order. OpenBISS will bind
// to the first available one to allow side-by-side coexistence during migration.
var BISSPorts = []int{53952, 53953, 53954, 53955}

// Config holds all runtime configuration for OpenBISS.
type Config struct {
	// DataDir is the directory where TLS certificates and state are stored.
	// Defaults to ~/.openbiss on macOS/Linux and %APPDATA%\OpenBISS on Windows.
	DataDir string `json:"data_dir"`

	// PKCS11Lib is an optional override for the PKCS#11 shared library path.
	// When empty, OpenBISS auto-discovers platform-specific libraries.
	// Set via OPENBISS_PKCS11_LIB environment variable.
	PKCS11Lib string `json:"pkcs11_lib"`

	// LogLevel controls the slog output verbosity ("debug", "info", "warn", "error").
	// Defaults to "info". Set via OPENBISS_LOG_LEVEL environment variable.
	LogLevel string `json:"log_level"`

	// Lang is the UI language code ("en" or "bg").
	// Auto-detected from OS locale; override with OPENBISS_LANG environment variable.
	Lang string `json:"lang"`
}

// Load resolves Config in three layers, in increasing order of precedence:
//
//  1. Built-in defaults (DataDir from defaultDataDir, LogLevel "info",
//     Lang auto-detected from OS locale).
//  2. $DataDir/config.json (if it exists and parses successfully). Fields
//     present in the JSON override defaults; fields absent keep defaults.
//  3. Environment variables (OPENBISS_DATA_DIR, OPENBISS_PKCS11_LIB,
//     OPENBISS_LOG_LEVEL, OPENBISS_LANG). Env vars always win.
//
// Bootstrap note: the JSON file lives inside DataDir, but DataDir is itself
// configurable. The loader resolves DataDir from env or defaults first, uses
// that path to locate config.json, and then re-applies env vars at the end.
//
// Missing config.json: silently continue with defaults (Load does not create
// the file — only Save does).
//
// Corrupt config.json: a warning is logged, the file is renamed to
// config.json.broken-<UTC timestamp>, and Load falls back to defaults.
func Load() (*Config, error) {
	defaultDir, err := defaultDataDir()
	if err != nil {
		return nil, err
	}

	bootstrapDataDir := envOrDefault("OPENBISS_DATA_DIR", defaultDir)
	cfg := &Config{
		DataDir:   bootstrapDataDir,
		PKCS11Lib: "",
		LogLevel:  "info",
		Lang:      i18n.DetectLanguage(),
	}

	jsonPath := filepath.Join(bootstrapDataDir, "config.json")
	loadJSONOverlay(cfg, jsonPath)

	if v := os.Getenv("OPENBISS_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("OPENBISS_PKCS11_LIB"); v != "" {
		cfg.PKCS11Lib = v
	}
	if v := os.Getenv("OPENBISS_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("OPENBISS_LANG"); v != "" {
		cfg.Lang = v
	}

	// Ensure the data directory exists so TLS cert generation has a place to write.
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadJSONOverlay reads jsonPath and unmarshals it on top of cfg. Missing
// files are silently tolerated. Corrupt files are quarantined by renaming to
// jsonPath + ".broken-<timestamp>" and cfg is reset to its pre-overlay values
// so partial JSON parsing cannot leak into the loaded config.
func loadJSONOverlay(cfg *Config, jsonPath string) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("could not read config.json; using defaults",
				"path", jsonPath, "error", err)
		}
		return
	}

	preOverlay := *cfg
	if err := json.Unmarshal(data, cfg); err != nil {
		brokenPath := jsonPath + ".broken-" + time.Now().UTC().Format("20060102T150405Z")
		slog.Warn("corrupt config.json; renaming and using defaults",
			"path", jsonPath, "broken_path", brokenPath, "error", err)
		if renameErr := os.Rename(jsonPath, brokenPath); renameErr != nil {
			slog.Warn("failed to rename corrupt config.json",
				"path", jsonPath, "error", renameErr)
		}
		*cfg = preOverlay
	}
}

// Save persists the Config to $DataDir/config.json using an atomic
// tmp+rename write. Callers (e.g. the first-run wizard and the settings
// screen) use this to make changes durable across restarts.
func (c *Config) Save() error {
	return SaveAtomic(filepath.Join(c.DataDir, "config.json"), c)
}

// TLSCertPath returns the path to the self-signed TLS certificate PEM file.
func (c *Config) TLSCertPath() string {
	return filepath.Join(c.DataDir, "localhost.crt")
}

// TLSKeyPath returns the path to the self-signed TLS private key PEM file.
func (c *Config) TLSKeyPath() string {
	return filepath.Join(c.DataDir, "localhost.key")
}

// defaultDataDir returns the platform-appropriate data directory.
//
//   - macOS / Linux: ~/.openbiss
//   - Windows: %APPDATA%\OpenBISS
func defaultDataDir() (string, error) {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "OpenBISS"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".openbiss"), nil
}

// envOrDefault returns the value of the named environment variable,
// falling back to fallback when the variable is unset or empty.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
