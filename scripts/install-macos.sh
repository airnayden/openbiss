#!/usr/bin/env bash
# OpenBISS macOS installer (source build).
#
# Builds OpenBISS from source as a proper macOS .app bundle (via `fyne
# package`) and installs it to /Applications or ~/Applications.
#
# Usage:
#   ./scripts/install-macos.sh              # build + install to /Applications (sudo if needed)
#   ./scripts/install-macos.sh --user       # build + install to ~/Applications (no sudo)
#   ./scripts/install-macos.sh --uninstall  # remove OpenBISS.app from both locations
#                                           # (preserves ~/.openbiss user data)
#
# Prerequisites (checked automatically):
#   - Go toolchain  (brew install go  or  https://go.dev/dl/)
#   - Xcode Command Line Tools  (xcode-select --install)
#   - fyne CLI is auto-installed via `go install` if missing
#
# This script never auto-launches the app, never code-signs or notarizes,
# and never touches ~/.openbiss user data.
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

APP_NAME="OpenBISS"
APP_BUNDLE="${APP_NAME}.app"
APP_ID="com.openbiss.openbiss"
ICON_PATH="assets/icon.png"
SYSTEM_INSTALL_DIR="/Applications"
USER_INSTALL_DIR="${HOME}/Applications"

MODE="install"
INSTALL_DIR="$SYSTEM_INSTALL_DIR"

case "${1:-}" in
    --user)
        INSTALL_DIR="$USER_INSTALL_DIR"
        ;;
    --uninstall)
        MODE="uninstall"
        ;;
    -h|--help)
        awk 'NR>1 && /^#/ {sub(/^# ?/, ""); print; next} NR>1 {exit}' "$0"
        exit 0
        ;;
    "")
        : # default: system-wide install via $SYSTEM_INSTALL_DIR
        ;;
    *)
        echo "❌ Unknown argument: $1" >&2
        echo "   Run '$0 --help' for usage." >&2
        exit 1
        ;;
esac

if [ "$MODE" = "uninstall" ]; then
    echo "🗑  Uninstalling OpenBISS…"

    if [ -d "${SYSTEM_INSTALL_DIR}/${APP_BUNDLE}" ]; then
        echo "   → Removing ${SYSTEM_INSTALL_DIR}/${APP_BUNDLE}"
        if [ -w "$SYSTEM_INSTALL_DIR" ]; then
            rm -rf "${SYSTEM_INSTALL_DIR}/${APP_BUNDLE}" || true
        else
            sudo rm -rf "${SYSTEM_INSTALL_DIR}/${APP_BUNDLE}" || true
        fi
    fi

    if [ -d "${USER_INSTALL_DIR}/${APP_BUNDLE}" ]; then
        echo "   → Removing ${USER_INSTALL_DIR}/${APP_BUNDLE}"
        rm -rf "${USER_INSTALL_DIR}/${APP_BUNDLE}" || true
    fi

    echo "✅ Uninstall complete."
    echo "   ℹ️  User data preserved at ~/.openbiss (TLS cert, logs, settings)."
    echo "   To remove it manually: rm -rf ~/.openbiss"
    exit 0
fi

echo "🔍 Checking prerequisites…"

if ! command -v go >/dev/null 2>&1; then
    echo "❌ Go toolchain not found." >&2
    echo "   → Install via Homebrew:  brew install go" >&2
    echo "   → Or download from:     https://go.dev/dl/" >&2
    exit 1
fi
echo "   ✓ Go found: $(go version)"

if ! xcode-select -p >/dev/null 2>&1; then
    echo "❌ Xcode Command Line Tools not found." >&2
    echo "   → Install with: xcode-select --install" >&2
    exit 1
fi
echo "   ✓ Xcode Command Line Tools found: $(xcode-select -p)"

FYNE_BIN="${HOME}/go/bin/fyne"
if [ ! -x "$FYNE_BIN" ]; then
    echo "📦 Installing fyne CLI (go install fyne.io/tools/cmd/fyne@latest)…"
    go install fyne.io/tools/cmd/fyne@latest
fi
if [ ! -x "$FYNE_BIN" ]; then
    echo "❌ fyne CLI install failed; expected at ${FYNE_BIN}" >&2
    exit 1
fi
echo "   ✓ fyne CLI: ${FYNE_BIN}"

if [ ! -f "$ICON_PATH" ]; then
    echo "❌ Missing icon: ${ICON_PATH}" >&2
    exit 1
fi
if [ ! -f "FyneApp.toml" ]; then
    echo "❌ Missing FyneApp.toml at project root." >&2
    exit 1
fi

rm -rf "$APP_BUNDLE"

echo "🔨 Building ${APP_BUNDLE} via fyne package…"
"$FYNE_BIN" package \
    -os darwin \
    -icon "$ICON_PATH" \
    -name "$APP_NAME" \
    -app-id "$APP_ID" \
    -release

if [ ! -d "$APP_BUNDLE" ]; then
    echo "❌ Build failed: ${APP_BUNDLE} was not produced." >&2
    exit 1
fi
echo "   ✓ Built ${APP_BUNDLE}"

# Strip Gatekeeper quarantine attribute so first launch doesn't trip the
# "downloaded from the internet" warning. -dr recurses; errors are ignored
# when the attribute is absent (e.g. on a freshly built bundle).
xattr -dr com.apple.quarantine "$APP_BUNDLE" 2>/dev/null || true

mkdir -p "$INSTALL_DIR"

TARGET="${INSTALL_DIR}/${APP_BUNDLE}"
echo "📁 Installing to ${TARGET}…"

if [ -d "$TARGET" ]; then
    echo "   → Removing existing ${TARGET}"
    if [ -w "$INSTALL_DIR" ]; then
        rm -rf "$TARGET"
    else
        sudo rm -rf "$TARGET"
    fi
fi

if [ -w "$INSTALL_DIR" ]; then
    mv "$APP_BUNDLE" "$TARGET"
else
    echo "   → ${INSTALL_DIR} requires elevated permissions; using sudo."
    sudo mv "$APP_BUNDLE" "$TARGET"
fi

echo "   ✓ Installed ${TARGET}"

cat <<EOF

✅ OpenBISS installed successfully.

To launch:
   • Spotlight:   ⌘+Space → "OpenBISS" → Return
   • Finder:      Open "$INSTALL_DIR" → double-click OpenBISS
   • Terminal:    open -a OpenBISS

First-launch notes:
   • macOS will prompt to trust the self-signed TLS certificate the first
     time OpenBISS starts. Approve it once; the cert is saved in ~/.openbiss
     and reused on subsequent launches.
   • If Gatekeeper blocks the app, right-click OpenBISS in Finder and choose
     "Open", then confirm. (Quarantine is already stripped from this build,
     but the warning may still appear once.)

To uninstall:
   $0 --uninstall

EOF
