#!/usr/bin/env bash
# ============================================================================
# OpenBISS Linux installer (source build)
# ----------------------------------------------------------------------------
# Builds OpenBISS from source with `go build` and installs it XDG-style:
# a raw ELF binary plus a `.desktop` launcher entry and a hicolor icon.
# No `.app` bundles, no `fyne package` — Linux convention is separate files
# under share/applications, share/icons, and bin.
#
# Usage:
#   ./scripts/install-linux.sh              # build + install to ~/.local (no sudo)
#   ./scripts/install-linux.sh --system     # build + install to /usr/local (sudo)
#   ./scripts/install-linux.sh --uninstall  # remove from BOTH prefixes
#                                           # (preserves ~/.openbiss user data)
#
# Prerequisites (checked automatically, with per-distro install hints):
#   - Go toolchain
#   - C compiler (gcc) — Fyne uses cgo
#   - pkg-config
#   - OpenGL headers (libgl-dev / mesa-libGL-devel / mesa)
#
# This script never auto-launches the app and never touches ~/.openbiss
# user data (TLS cert, logs, settings).
# ============================================================================
set -euo pipefail

# ---------------------------------------------------------------------------
# Resolve project root so the script works from any cwd.
# ---------------------------------------------------------------------------
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

APP_NAME="OpenBISS"
BINARY_NAME="openbiss"
APP_ID="com.openbiss.openbiss"
ICON_SRC="assets/icon.png"
MODULE_PATH="github.com/openbiss/openbiss"

USER_PREFIX="${HOME}/.local"
SYSTEM_PREFIX="/usr/local"

# ---------------------------------------------------------------------------
# Parse flags.
# ---------------------------------------------------------------------------
MODE="install"
PREFIX="$USER_PREFIX"
SUDO=""

case "${1:-}" in
    --system)
        PREFIX="$SYSTEM_PREFIX"
        SUDO="sudo"
        ;;
    --uninstall)
        MODE="uninstall"
        ;;
    -h|--help)
        sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
        exit 0
        ;;
    "")
        # Default: user-local install.
        ;;
    *)
        echo "❌ Unknown argument: $1" >&2
        echo "   Run '$0 --help' for usage." >&2
        exit 1
        ;;
esac

# ---------------------------------------------------------------------------
# Helper: compute install paths for a given prefix.
# ---------------------------------------------------------------------------
paths_for_prefix() {
    local p="$1"
    echo "${p}/bin/${BINARY_NAME}"
    echo "${p}/share/applications/${BINARY_NAME}.desktop"
    echo "${p}/share/icons/hicolor/512x512/apps/${BINARY_NAME}.png"
}

# ---------------------------------------------------------------------------
# Uninstall path: remove from BOTH prefixes (best-effort), preserve user data.
# ---------------------------------------------------------------------------
if [ "$MODE" = "uninstall" ]; then
    echo "🗑  Uninstalling OpenBISS…"

    for p in "$USER_PREFIX" "$SYSTEM_PREFIX"; do
        bin="${p}/bin/${BINARY_NAME}"
        desktop="${p}/share/applications/${BINARY_NAME}.desktop"
        icon="${p}/share/icons/hicolor/512x512/apps/${BINARY_NAME}.png"

        # Choose sudo only when the prefix needs it AND the file exists.
        if [ "$p" = "$SYSTEM_PREFIX" ]; then
            S="sudo"
        else
            S=""
        fi

        if [ -e "$bin" ] || [ -L "$bin" ]; then
            echo "   → Removing $bin"
            $S rm -f "$bin" 2>/dev/null || true
        fi
        if [ -e "$desktop" ]; then
            echo "   → Removing $desktop"
            $S rm -f "$desktop" 2>/dev/null || true
        fi
        if [ -e "$icon" ]; then
            echo "   → Removing $icon"
            $S rm -f "$icon" 2>/dev/null || true
        fi

        # Refresh desktop/icon caches (best-effort).
        if [ -d "${p}/share/applications" ]; then
            $S update-desktop-database "${p}/share/applications" 2>/dev/null || true
        fi
        if [ -d "${p}/share/icons/hicolor" ]; then
            $S gtk-update-icon-cache -f -t "${p}/share/icons/hicolor" 2>/dev/null || true
        fi
    done

    echo "✅ Uninstall complete."
    echo "   ℹ️  User data preserved at ~/.openbiss (TLS cert, logs, settings)."
    echo "   To remove it manually: rm -rf ~/.openbiss"
    exit 0
fi

# ---------------------------------------------------------------------------
# Prerequisite checks with per-distro install hints.
# ---------------------------------------------------------------------------
print_distro_hints() {
    local pkg_debian="$1"
    local pkg_fedora="$2"
    local pkg_arch="$3"
    echo "   Install hints:" >&2
    echo "     Debian/Ubuntu:  sudo apt install ${pkg_debian}" >&2
    echo "     Fedora/RHEL:    sudo dnf install ${pkg_fedora}" >&2
    echo "     Arch/Manjaro:   sudo pacman -S ${pkg_arch}" >&2
}

echo "🔍 Checking prerequisites…"

MISSING=0

if ! command -v go >/dev/null 2>&1; then
    echo "❌ Go toolchain not found." >&2
    print_distro_hints "golang" "golang" "go"
    echo "     Or download from: https://go.dev/dl/" >&2
    MISSING=1
else
    echo "   ✓ Go found: $(go version)"
fi

if ! command -v gcc >/dev/null 2>&1; then
    echo "❌ gcc (C compiler) not found — Fyne uses cgo and needs it." >&2
    print_distro_hints "build-essential" "gcc make" "base-devel"
    MISSING=1
else
    echo "   ✓ gcc found: $(gcc --version | head -n1)"
fi

if ! command -v pkg-config >/dev/null 2>&1; then
    echo "❌ pkg-config not found." >&2
    print_distro_hints "pkg-config" "pkgconf-pkg-config" "pkgconf"
    MISSING=1
else
    echo "   ✓ pkg-config found: $(pkg-config --version)"
fi

# OpenGL headers: accept either desktop GL or GLES.
if command -v pkg-config >/dev/null 2>&1; then
    if pkg-config --exists gl 2>/dev/null; then
        echo "   ✓ OpenGL headers found (gl)"
    elif pkg-config --exists glesv2 2>/dev/null; then
        echo "   ✓ OpenGL ES headers found (glesv2)"
    else
        echo "❌ OpenGL headers not found (neither gl nor glesv2)." >&2
        echo "   Also commonly needed: xorg / X11 / xcursor / xrandr / xinerama / xi / xxf86vm headers." >&2
        print_distro_hints \
            "libgl1-mesa-dev xorg-dev" \
            "mesa-libGL-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel" \
            "mesa libxcursor libxrandr libxinerama libxi"
        MISSING=1
    fi
fi

if [ "$MISSING" -ne 0 ]; then
    echo "" >&2
    echo "❌ One or more prerequisites are missing. Install them and re-run." >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Sanity-check inputs.
# ---------------------------------------------------------------------------
if [ ! -f "$ICON_SRC" ]; then
    echo "❌ Missing icon: ${ICON_SRC}" >&2
    exit 1
fi
if [ ! -f "go.mod" ]; then
    echo "❌ Missing go.mod at project root: $PROJECT_ROOT" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Build.
# ---------------------------------------------------------------------------
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo "dev")"
echo "🔨 Building ${BINARY_NAME} (version=${VERSION})…"

go build \
    -ldflags "-s -w -X ${MODULE_PATH}/internal/config.Version=${VERSION}" \
    -o "${BINARY_NAME}" \
    .

if [ ! -x "${BINARY_NAME}" ]; then
    echo "❌ Build failed: ${BINARY_NAME} not produced." >&2
    exit 1
fi
echo "   ✓ Built ${BINARY_NAME}"

# ---------------------------------------------------------------------------
# Compute install paths and ensure target dirs exist.
# ---------------------------------------------------------------------------
BIN_DIR="${PREFIX}/bin"
APP_DIR="${PREFIX}/share/applications"
ICON_DIR="${PREFIX}/share/icons/hicolor/512x512/apps"

BIN_FILE="${BIN_DIR}/${BINARY_NAME}"
DESKTOP_FILE="${APP_DIR}/${BINARY_NAME}.desktop"
ICON_FILE="${ICON_DIR}/${BINARY_NAME}.png"

echo "📁 Installing to prefix: ${PREFIX}"
echo "   → ${BIN_FILE}"
echo "   → ${DESKTOP_FILE}"
echo "   → ${ICON_FILE}"

$SUDO mkdir -p "$BIN_DIR" "$APP_DIR" "$ICON_DIR"

# ---------------------------------------------------------------------------
# Install binary (mode 755) and icon (mode 644).
# ---------------------------------------------------------------------------
$SUDO install -m 755 "${BINARY_NAME}" "${BIN_FILE}"
$SUDO install -m 644 "${ICON_SRC}" "${ICON_FILE}"

# ---------------------------------------------------------------------------
# Write .desktop launcher entry.
# Uses `$SUDO tee` so the heredoc works for both user-local and system installs.
# ---------------------------------------------------------------------------
$SUDO tee "${DESKTOP_FILE}" >/dev/null <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=OpenBISS
GenericName=Smart-card Signing Service
Comment=Open-source BORICA BISS replacement for КЕП signing
Exec=${BIN_FILE}
Icon=${BINARY_NAME}
Terminal=false
Categories=Utility;Security;Office;
Keywords=BISS;BORICA;PKCS11;smartcard;signing;КЕП;НЗИС;
StartupWMClass=${APP_ID}
EOF

$SUDO chmod 644 "${DESKTOP_FILE}"

# ---------------------------------------------------------------------------
# Refresh desktop & icon caches (best-effort — fine if these binaries are
# missing, the entries still work after the next session/login).
# ---------------------------------------------------------------------------
$SUDO update-desktop-database "${APP_DIR}" 2>/dev/null || true
$SUDO gtk-update-icon-cache -f -t "${PREFIX}/share/icons/hicolor" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Clean up the in-tree build artifact.
# ---------------------------------------------------------------------------
rm -f "${PROJECT_ROOT}/${BINARY_NAME}"

# ---------------------------------------------------------------------------
# PATH advisory for user-local installs.
# ---------------------------------------------------------------------------
PATH_WARNING=""
if [ "$PREFIX" = "$USER_PREFIX" ]; then
    case ":${PATH}:" in
        *":${BIN_DIR}:"*)
            : # already on PATH
            ;;
        *)
            PATH_WARNING="yes"
            ;;
    esac
fi

# ---------------------------------------------------------------------------
# Final report.
# ---------------------------------------------------------------------------
cat <<EOF

✅ OpenBISS installed successfully.

Launch:
   • App menu / activities:  search for "OpenBISS"
   • Terminal:               ${BINARY_NAME}
   • Direct path:            ${BIN_FILE}

First-launch notes:
   • A self-signed TLS certificate is generated under ~/.openbiss on first
     start; browsers will warn until you trust it.
EOF

if [ -n "$PATH_WARNING" ]; then
    cat <<EOF

⚠️  PATH notice
   ${BIN_DIR} is NOT on your \$PATH. To run \`${BINARY_NAME}\` directly, add:

       export PATH="\$HOME/.local/bin:\$PATH"

   to your shell profile (~/.bashrc, ~/.zshrc, ~/.profile, …).
   Until then, use the full path: ${BIN_FILE}
EOF
fi

cat <<EOF

System-tray caveat (Linux):
   GNOME does NOT show legacy tray icons out of the box. Install the
   "AppIndicator and KStatusNotifierItem Support" GNOME extension if you
   want the OpenBISS tray icon to appear. On KDE Plasma, XFCE, Cinnamon,
   MATE and most other DEs the tray works without extra setup.

To uninstall:
   $0 --uninstall

EOF
