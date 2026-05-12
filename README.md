# OpenBISS

Open-source replacement for BORICA's BISS (Browser Independent Signing Service) written in Go.

BISS is a closed-source Java application used in Bulgaria's health system (НЗИС), e-prescriptions, and dental reporting. It runs as a local HTTPS server on ports 53952–53955 and enables browsers to sign documents using smart cards (КЕП / qualified electronic signatures) via PKCS#11.

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

## Install (one command)

### macOS

```bash
git clone https://github.com/openbiss/openbiss.git && cd openbiss
./scripts/install-macos.sh --user    # installs to ~/Applications
# or: ./scripts/install-macos.sh    # installs to /Applications (may prompt for sudo)
```

Launch from Launchpad, or run `open ~/Applications/OpenBISS.app`. First launch
asks for your password once to trust the local TLS certificate.

### Linux

```bash
git clone https://github.com/openbiss/openbiss.git && cd openbiss
./scripts/install-linux.sh           # installs to ~/.local
# or: ./scripts/install-linux.sh --system    # installs to /usr/local (requires sudo)
```

Launch from your application menu (search "OpenBISS") or run `openbiss` in a terminal.

### Windows

```powershell
git clone https://github.com/openbiss/openbiss.git
cd openbiss
.\scripts\install-windows.ps1
```

Launch from Start Menu (search "OpenBISS"). First launch shows a SmartScreen
warning — click "More info" → "Run anyway". See `docs/SECURITY-WARNINGS.md`.

**Uninstall any platform** with the same script and `--uninstall` (or `-Uninstall` on Windows).
Your config and TLS certificate in `~/.openbiss` (`%APPDATA%\OpenBISS` on Windows) are preserved.

## Manual Installation

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/openbiss/openbiss/main/tools/openbiss/scripts/install.sh | bash
```

Or manually:

```bash
# Build from source
cd tools/openbiss
make build-darwin        # Intel Mac
make build-darwin-arm    # Apple Silicon
make build-linux         # Linux x86-64

# Copy to PATH
cp dist/openbiss-darwin-arm64 /usr/local/bin/openbiss
chmod +x /usr/local/bin/openbiss
```

### Windows

```powershell
# Build from source
cd tools\openbiss
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
| **API** | Per-endpoint request counters (Version / GetSigner / Sign), success rate, and a scrollable recent-requests list |
| **Settings** | Language, log level, PKCS#11 library path, autostart toggle, and TLS certificate regeneration |
| **Logs** | Live scrolling log viewer with Clear button (ring buffer, last 1000 entries) |
| **Certificates** | Smart card certificates with CN, issuer, and expiry date; Refresh button |
| **About** | Version, bundle ID, GitHub link, license, and known-behavior disclosures |

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

### Key Difference from BISS

The `/sign` endpoint validates `signedContentsCert` against the **OS trust store** rather than BISS's custom B-Trust-only trust store. This means:

- ✅ Certificates signed by any trusted CA (Let's Encrypt, DigiCert, etc.) are accepted
- ✅ Enterprise internal CAs trusted by the OS are accepted  
- ❌ Certificates NOT trusted by the OS are rejected (prevents MITM)

## Building

```bash
cd tools/openbiss
make build-all    # all platforms
make build        # current platform
make vet          # go vet
```

## Building Release Bundles

Release bundles (macOS `.app`, Windows `.exe` with icon, Linux tarball) are built with [fyne-cross](https://github.com/fyne-io/fyne-cross), which requires Docker.

```bash
# Install fyne-cross (once)
go install github.com/fyne-io/fyne-cross@latest

# Build all platform bundles
make package-all
```

`make package-all` produces:

| Output | Platform |
|---|---|
| `dist/OpenBISS-darwin-amd64.app` | macOS Intel |
| `dist/OpenBISS-darwin-arm64.app` | macOS Apple Silicon |
| `dist/OpenBISS-windows-amd64.exe` | Windows x86-64 |
| `dist/openbiss-linux-amd64.tar.gz` | Linux x86-64 |

Bundles embed the app icon (`assets/icon.png`) and `FyneApp.toml` metadata. They are **not code-signed**. See [docs/SECURITY-WARNINGS.md](docs/SECURITY-WARNINGS.md) for Gatekeeper and SmartScreen bypass instructions.

## License

MIT
