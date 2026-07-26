# OpenBISS

[![Build](https://github.com/airnayden/openbiss/actions/workflows/build.yml/badge.svg?branch=main)](https://github.com/airnayden/openbiss/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/airnayden/openbiss?display_name=tag&sort=semver)](https://github.com/airnayden/openbiss/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/airnayden/openbiss.svg)](https://pkg.go.dev/github.com/airnayden/openbiss)

Open-source replacement for BORICA's BISS (Browser Independent Signing Service) written in Go.

BISS is a closed-source Java application used in Bulgaria's health system (НЗИС), e-prescriptions, and dental reporting. It runs as a local HTTPS server on ports 53952–53955 and enables browsers to sign documents using smart cards (КЕП / qualified electronic signatures) via PKCS#11.

## Table of Contents

- [Why OpenBISS?](#why-openbiss)
- [Download a Prebuilt Binary](#download-a-prebuilt-binary)
- [Install (one command)](#install-one-command)
- [Manual Installation](#manual-installation)
- [Usage](#usage)
- [Desktop GUI](#desktop-gui)
- [API Compatibility](#api-compatibility)
- [Building from Source](#building-from-source)
- [Continuous Integration](#continuous-integration)
- [Contributing](#contributing)
- [License](#license)

## Why OpenBISS?

| | BISS | OpenBISS |
|---|---|---|
| Size | 200 MB+ (JRE bundled) | ~8 MB single binary |
| Source | Closed / ProGuard obfuscated | Open source (MIT) |
| Trust store | B-Trust CA only | OS trust store (macOS Keychain / Windows CertStore / Linux CA bundle) |
| PKCS#11 libs | Hardcoded | Auto-discovered + `OPENBISS_PKCS11_LIB` override |
| Languages | Bulgarian only | English + Bulgarian (auto-detect + override) |
| PIN logging | Unknown | Never logged or stored |
| GUI | Limited Java UI | Full native Fyne desktop app (6 tabs) |
| Distribution | Vendor-only installer | GitHub Releases + CI-built binaries for Linux/macOS/Windows |

## Download a Prebuilt Binary

Every push to `main` builds artifacts for all three platforms via [GitHub Actions](.github/workflows/build.yml). Every tagged release (`v*`) uploads signed-by-checksum binaries to GitHub Releases.

Grab the latest from [Releases](https://github.com/airnayden/openbiss/releases/latest):

| Platform | Asset |
|---|---|
| Linux (x86-64) | `openbiss-linux-amd64` |
| macOS (Intel) | `openbiss-darwin-amd64` |
| macOS (Apple Silicon) | `openbiss-darwin-arm64` |
| Windows (x86-64) | `openbiss-windows-amd64.exe` |

Each asset is accompanied by `<asset>.sha256` for integrity verification.

```bash
# Linux / macOS — replace VERSION and PLATFORM as needed
VERSION=v0.1.0
PLATFORM=linux-amd64        # or darwin-amd64 / darwin-arm64
curl -fsSLO "https://github.com/airnayden/openbiss/releases/download/${VERSION}/openbiss-${PLATFORM}"
curl -fsSLO "https://github.com/airnayden/openbiss/releases/download/${VERSION}/openbiss-${PLATFORM}.sha256"
shasum -a 256 -c "openbiss-${PLATFORM}.sha256"
chmod +x "openbiss-${PLATFORM}"
sudo mv "openbiss-${PLATFORM}" /usr/local/bin/openbiss
openbiss --headless        # or just `openbiss` for the GUI
```

```powershell
# Windows (PowerShell)
$Version = "v0.1.0"
$Asset   = "openbiss-windows-amd64.exe"
Invoke-WebRequest "https://github.com/airnayden/openbiss/releases/download/$Version/$Asset" -OutFile $Asset
Invoke-WebRequest "https://github.com/airnayden/openbiss/releases/download/$Version/$Asset.sha256" -OutFile "$Asset.sha256"
# Verify
$expected = (Get-Content "$Asset.sha256").Split(' ')[0]
$actual   = (Get-FileHash $Asset -Algorithm SHA256).Hash.ToLower()
if ($expected -ne $actual) { throw "Checksum mismatch" }
.\$Asset
```

Pre-built binaries are **not code-signed**. See [Security Warnings](#security-warnings-unsigned-binaries) below.

## Install (one command)

### macOS

```bash
git clone https://github.com/airnayden/openbiss.git && cd openbiss
./scripts/install-macos.sh --user    # installs to ~/Applications
# or: ./scripts/install-macos.sh    # installs to /Applications (may prompt for sudo)
```

Launch from Launchpad, or run `open ~/Applications/OpenBISS.app`. First launch
asks for your password once to trust the local TLS certificate.

### Linux

```bash
git clone https://github.com/airnayden/openbiss.git && cd openbiss
./scripts/install-linux.sh           # installs to ~/.local
# or: ./scripts/install-linux.sh --system    # installs to /usr/local (requires sudo)
```

Launch from your application menu (search "OpenBISS") or run `openbiss` in a terminal.

### Windows

```powershell
git clone https://github.com/airnayden/openbiss.git
cd openbiss
.\scripts\install-windows.ps1
```

Launch from Start Menu (search "OpenBISS"). First launch shows a SmartScreen
warning — click "More info" → "Run anyway". See `docs/SECURITY-WARNINGS.md`.

**Uninstall any platform** with the same script and `--uninstall` (or `-Uninstall` on Windows).
Your config and TLS certificate in `~/.openbiss` (`%APPDATA%\OpenBISS` on Windows) are preserved.

## Manual Installation

### macOS / Linux (binary install via the bundled installer)

```bash
curl -fsSL https://raw.githubusercontent.com/airnayden/openbiss/main/scripts/install.sh | bash
```

This installer downloads the latest GitHub Release binary for your platform, installs it to `/usr/local/bin/openbiss`, and registers a launchd / systemd unit so OpenBISS starts at login.

Or build manually:

```bash
git clone https://github.com/airnayden/openbiss.git && cd openbiss

make build-darwin        # Intel Mac     → dist/openbiss-darwin-amd64
make build-darwin-arm    # Apple Silicon → dist/openbiss-darwin-arm64
make build-linux         # Linux x86-64  → dist/openbiss-linux-amd64

sudo install -m 755 dist/openbiss-$(uname -s | tr A-Z a-z)-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') /usr/local/bin/openbiss
```

### Windows (build manually)

```powershell
git clone https://github.com/airnayden/openbiss.git
cd openbiss
make build-windows

# Run
.\dist\openbiss-windows-amd64.exe
```

### Security Warnings (Unsigned Binaries)

Pre-built binaries are not code-signed. Your OS will warn you on first launch.

**macOS (Gatekeeper):** Right-click the binary in Finder and choose **Open**, then confirm in the dialog. Alternatively, from Terminal:

```bash
xattr -d com.apple.quarantine ./openbiss
```

**Windows (SmartScreen):** Click **More info** in the SmartScreen dialog, then **Run anyway**.

See [docs/SECURITY-WARNINGS.md](docs/SECURITY-WARNINGS.md) for a full explanation of why these warnings appear and what they mean.

## Usage

```bash
openbiss
```

OpenBISS auto-selects the first available port from 53952–53955 and prints the URL:

```
INFO OpenBISS listening addr=https://127.0.0.1:53952
```

On first run a self-signed TLS certificate is generated and stored in `~/.openbiss/` (macOS/Linux) or `%APPDATA%\OpenBISS\` (Windows).

### Command-Line Flags

| Flag | Default | Description |
|---|---|---|
| `--lang` | _(auto-detect)_ | UI language: `en` or `bg`. Overrides `OPENBISS_LANG` and OS locale. |
| `--headless` | `false` | Run as a background server without any GUI. |

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `OPENBISS_PKCS11_LIB` | _(auto-discover)_ | Path to PKCS#11 shared library |
| `OPENBISS_DATA_DIR` | `~/.openbiss` | Directory for TLS cert storage |
| `OPENBISS_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `OPENBISS_LANG` | _(auto-detect)_ | UI language: `en` or `bg` (overrides OS locale) |

### Language Selection

OpenBISS supports English (`en`) and Bulgarian (`bg`) for logs, PIN prompts, certificate selection dialogs, and error messages. The language is resolved in this order:

1. `--lang` CLI flag
2. `OPENBISS_LANG` environment variable
3. OS locale (macOS `AppleLocale`, Windows culture, Linux `LANG`)
4. English fallback

```bash
# macOS / Linux
openbiss --lang bg
OPENBISS_LANG=bg openbiss

# Windows (PowerShell)
.\openbiss.exe --lang bg
$env:OPENBISS_LANG="bg"; .\openbiss.exe
```

### Supported Smart Card Middleware

OpenBISS auto-discovers these libraries (platform-specific):

- **SafeNet/Thales eToken** (`libIDPrimePKCS11.dylib` / `eTPKCS11.dll`)  
- **OpenSC** (`opensc-pkcs11.so` / `opensc-pkcs11.dll`)

To use a different library:

```bash
OPENBISS_PKCS11_LIB=/path/to/your-pkcs11.so openbiss
```

## Desktop GUI

OpenBISS ships as a full desktop app with a 6-tab window and an in-app first-run wizard.

Screenshots: [`./docs/screenshots/`](docs/screenshots/)

### Launch

```bash
./openbiss
```

Opens the main window. The server starts automatically in the background. On first launch, a wizard confirms your language preference and detects your smart card reader.

Closing the window quits OpenBISS (the local HTTPS server stops along with the app).

### Window Tabs

| Tab | What it shows |
|---|---|
| **Status** | Server state (Running/Stopped), port, PKCS#11 driver, certificate count, uptime, and Start/Stop server buttons |
| **API** | Per-endpoint request counters (Version / GetSigner / Sign), success rate, a numbered list of recent requests, and a **request detail panel** showing the selected request's method, path, status, duration, full request + response headers, and bounded request + response bodies (cap 64 KiB / direction; `Authorization`, `Cookie`, `Set-Cookie`, `Proxy-Authorization` are redacted). Includes an **Open API Documentation** button that opens an interactive Swagger UI at `https://127.0.0.1:<port>/docs` (vendored, works offline) |
| **Settings** | Language, log level, PKCS#11 library path, autostart toggle, and TLS certificate regeneration |
| **Logs** | Live scrolling log viewer with Clear button (ring buffer, last 1000 entries) |
| **Certificates** | Smart card certificates with CN, issuer, and expiry date; Refresh button |
| **About** | App icon, version, bundle ID, and author. Fully static by design — opening this tab makes no network calls (no update probe, no telemetry, no version check) |

### Headless / CLI Mode

```bash
./openbiss --headless
```

Runs as a background server without any GUI. Ideal for SSH sessions, CI pipelines, or when using the macOS launchd / Linux systemd service installed by `install.sh`. All existing behavior is preserved; native OS dialogs handle PIN prompts and certificate selection.

### Autostart on Login

Open **Settings** and toggle **Start at login**. This creates:

- macOS: a LaunchAgent plist at `~/Library/LaunchAgents/com.openbiss.openbiss.plist`
- Windows: a `HKCU\...\Run\OpenBISS` registry entry
- Linux: an XDG autostart `.desktop` file at `~/.config/autostart/openbiss.desktop`

The autostart entry launches OpenBISS in GUI mode at login.

## API Compatibility

OpenBISS implements the full BISS HTTP API:

| Endpoint | Method | Description |
|---|---|---|
| `/version` | GET | Capability document |
| `/getsigner` | POST | List certs, show selection dialog |
| `/sign` | POST | Sign content with smart card |

A machine-readable OpenAPI 3.0 specification is available at `https://127.0.0.1:<port>/openapi.json`. An interactive Swagger UI is served at `https://127.0.0.1:<port>/docs`. Both are vendored into the binary so they work without internet access.

### Key Difference from BISS

The `/sign` endpoint validates `signedContentsCert` against the **OS trust store** rather than BISS's custom B-Trust-only trust store. This means:

- ✅ Certificates signed by any trusted CA (Let's Encrypt, DigiCert, etc.) are accepted
- ✅ Enterprise internal CAs trusted by the OS are accepted  
- ❌ Certificates NOT trusted by the OS are rejected (prevents MITM)

## Building from Source

```bash
make build        # current platform → ./openbiss
make build-all    # cross-compile to dist/openbiss-{linux,darwin,windows}-*
make vet          # go vet ./...
make clean        # remove ./openbiss and dist/
```

CGO is required (Fyne uses GL bindings). On macOS install Xcode Command Line Tools (`xcode-select --install`); on Linux install the OpenGL + X11 dev headers (the [install-linux.sh](scripts/install-linux.sh) script will tell you the exact package names per distro); on Windows install MinGW-w64.

### Release Bundles (GUI installers)

Release bundles (macOS `.app`, Windows `.exe` with icon, Linux tarball) are built with [fyne-cross](https://github.com/fyne-io/fyne-cross), which requires Docker.

```bash
make fyne-deps     # one-time install of fyne + fyne-cross
make package-all   # produces fyne-cross/dist/{darwin,windows,linux}/
```

`make package-all` produces:

| Output | Platform |
|---|---|
| `OpenBISS-darwin-amd64.app` | macOS Intel |
| `OpenBISS-darwin-arm64.app` | macOS Apple Silicon |
| `OpenBISS-windows-amd64.exe` | Windows x86-64 |
| `openbiss-linux-amd64.tar.gz` | Linux x86-64 |

Bundles embed the app icon (`assets/icon.png`) and `FyneApp.toml` metadata. They are **not code-signed**. See [docs/SECURITY-WARNINGS.md](docs/SECURITY-WARNINGS.md) for Gatekeeper and SmartScreen bypass instructions.

## Continuous Integration

| Workflow | Trigger | What it does |
|---|---|---|
| [`build.yml`](.github/workflows/build.yml) | every push to `main`, every pull request, manual dispatch | `go vet` + `go build` on Ubuntu, macOS Intel, macOS Apple Silicon and Windows runners; uploads each native binary as a workflow artifact (14-day retention) |
| [`release.yml`](.github/workflows/release.yml) | every tag matching `v*`, or manual dispatch with a tag input | cross-platform native builds + SHA-256 checksums; uploads `openbiss-<platform>` + `openbiss-<platform>.sha256` to the matching GitHub Release; auto-generates release notes from commits |

The matrix covers four targets:

| Runner | GOOS / GOARCH | Artifact |
|---|---|---|
| `ubuntu-latest` | `linux/amd64` | `openbiss-linux-amd64` |
| `macos-13` | `darwin/amd64` | `openbiss-darwin-amd64` |
| `macos-latest` | `darwin/arm64` | `openbiss-darwin-arm64` |
| `windows-latest` | `windows/amd64` | `openbiss-windows-amd64.exe` |

Each binary is built **natively** on its target OS (no cross-compilation), so the Fyne CGo + OpenGL toolchain is satisfied on every runner. Build hardening flags: `-s -w` (strip + omit DWARF) and, on Windows, `-H windowsgui` so launching the binary doesn't spawn a console window.

### Cutting a Release

```bash
git tag -a v0.1.0 -m "OpenBISS v0.1.0"
git push origin v0.1.0
```

The `release.yml` workflow takes over: it builds all four targets, computes SHA-256 checksums, and uploads everything to the GitHub Release for tag `v0.1.0`.

## Contributing

Bug reports and pull requests are welcome at [github.com/airnayden/openbiss/issues](https://github.com/airnayden/openbiss/issues). The CI matrix above must stay green for any merge to `main`.

## License

MIT
