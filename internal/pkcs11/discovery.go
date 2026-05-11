// Package pkcs11 provides PKCS#11 smart card integration for OpenBISS.
// It uses github.com/miekg/pkcs11 which is a pure-Go PKCS#11 wrapper that
// loads the shared library at runtime via dlopen/LoadLibrary — no cgo needed.
package pkcs11

import (
	"os"
	"runtime"
)

// platformLibraries returns the ordered list of PKCS#11 shared library paths to
// probe on the current platform.
//
// The list is intentionally broad: OpenBISS tries each path and uses the first
// one that loads successfully. The order roughly reflects market prevalence in
// Bulgaria's smart card ecosystem:
//
//  1. SafeNet/Thales eToken (most common for B-Trust КЕП tokens)
//  2. OpenSC (universal open-source driver)
//  3. Additional vendor-specific drivers
var platformLibraries = map[string][]string{
	"darwin": {
		// SafeNet eToken / Thales IDPrime — standard B-Trust hardware token on macOS.
		"/Library/Frameworks/eToken.framework/Versions/A/libIDPrimePKCS11.dylib",
		// SafeNet Authentication Client legacy path.
		"/usr/local/lib/libeTPkcs11.dylib",
		// OpenSC — universal open-source PKCS#11 driver, supports most smart cards.
		"/usr/local/lib/opensc-pkcs11.so",
		// Homebrew-installed OpenSC.
		"/opt/homebrew/lib/opensc-pkcs11.so",
		// macOS built-in PCSC bridge (token-specific, not always PKCS#11).
		"/usr/lib/pkcs11/opensc-pkcs11.so",
		// PKCS#11 Spy — OpenSC debugging shim (lower priority).
		"/usr/local/lib/pkcs11-spy.so",
	},
	"windows": {
		// SafeNet eToken / Thales IDPrime — standard Windows path.
		`C:\Windows\System32\eTPKCS11.dll`,
		// SafeNet Authentication Client alternative name.
		`C:\Windows\System32\IDPrimePKCS11.dll`,
		// OpenSC for Windows (64-bit).
		`C:\Windows\System32\opensc-pkcs11.dll`,
		// OpenSC for Windows (32-bit compatibility layer).
		`C:\Windows\SysWOW64\opensc-pkcs11.dll`,
		// Program Files installation path for some vendors.
		`C:\Program Files\OpenSC Project\OpenSC\pkcs11\opensc-pkcs11.dll`,
	},
	"linux": {
		// OpenSC system package — most common on Debian/Ubuntu/Fedora.
		"/usr/lib/opensc-pkcs11.so",
		"/usr/lib/x86_64-linux-gnu/opensc-pkcs11.so",
		"/usr/lib/aarch64-linux-gnu/opensc-pkcs11.so",
		"/usr/lib64/opensc-pkcs11.so",
		// SafeNet eToken on Linux.
		"/usr/lib/libeTPkcs11.so",
		"/usr/local/lib/libeTPkcs11.so",
		// PKCS#11 Spy shim.
		"/usr/lib/x86_64-linux-gnu/pkcs11-spy.so",
	},
}

// DiscoverLibraries returns PKCS#11 shared library paths to try, in priority order.
//
// When OPENBISS_PKCS11_LIB is set it takes absolute precedence — only that path
// is returned. This lets sysadmins pin a specific library without modifying source.
func DiscoverLibraries(overridePath string) []string {
	if overridePath != "" {
		return []string{overridePath}
	}

	libs := platformLibraries[runtime.GOOS]
	return filterExisting(libs)
}

// filterExisting removes paths that do not exist on the filesystem.
// On Windows the file-existence check also covers the case where the DLL
// exists but cannot be loaded — the caller handles that separately.
func filterExisting(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			result = append(result, p)
		}
	}
	return result
}
