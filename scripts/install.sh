#!/usr/bin/env bash
# OpenBISS installer for macOS and Linux.
# Downloads the latest release binary, installs to /usr/local/bin, and sets up
# a launchd service on macOS or a systemd user service on Linux.
set -euo pipefail

REPO="airnayden/openbiss"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="openbiss"

detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"

    case "$arch" in
        x86_64)  arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *)
            echo "Unsupported architecture: $arch" >&2
            exit 1
            ;;
    esac

    echo "${os}-${arch}"
}

PLATFORM="$(detect_platform)"
ASSET_NAME="${BINARY_NAME}-${PLATFORM}"

echo "Detected platform: ${PLATFORM}"

# Resolve the latest release download URL from GitHub Releases.
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET_NAME}"

echo "Downloading OpenBISS from ${DOWNLOAD_URL}…"
curl -fsSL -o "/tmp/${BINARY_NAME}" "${DOWNLOAD_URL}"
chmod +x "/tmp/${BINARY_NAME}"

# Verify the binary runs before installing.
if ! "/tmp/${BINARY_NAME}" --help >/dev/null 2>&1; then
    echo "Binary smoke-test failed. Aborting installation." >&2
    exit 1
fi

echo "Installing to ${INSTALL_DIR}/${BINARY_NAME}…"
if [[ -w "${INSTALL_DIR}" ]]; then
    mv "/tmp/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    sudo mv "/tmp/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "OpenBISS installed successfully."

# macOS: register a launchd agent so OpenBISS starts at login.
if [[ "$(uname -s)" == "Darwin" ]]; then
    PLIST_DIR="${HOME}/Library/LaunchAgents"
    PLIST_PATH="${PLIST_DIR}/com.openbiss.openbiss.plist"

    mkdir -p "${PLIST_DIR}"
    cat > "${PLIST_PATH}" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.openbiss.openbiss</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${BINARY_NAME}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardErrorPath</key>
    <string>${HOME}/.openbiss/openbiss.log</string>
    <key>StandardOutPath</key>
    <string>${HOME}/.openbiss/openbiss.log</string>
</dict>
</plist>
PLIST

    launchctl load "${PLIST_PATH}" 2>/dev/null || true
    echo "launchd agent installed: ${PLIST_PATH}"
    echo "OpenBISS will start automatically at login."
fi

# Linux: register a systemd user service.
if [[ "$(uname -s)" == "Linux" ]] && command -v systemctl >/dev/null 2>&1; then
    SERVICE_DIR="${HOME}/.config/systemd/user"
    SERVICE_PATH="${SERVICE_DIR}/openbiss.service"

    mkdir -p "${SERVICE_DIR}"
    cat > "${SERVICE_PATH}" <<SERVICE
[Unit]
Description=OpenBISS - Open Source BISS Replacement
After=network.target

[Service]
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
SERVICE

    systemctl --user daemon-reload
    systemctl --user enable openbiss.service
    systemctl --user start openbiss.service
    echo "systemd user service installed and started."
fi

echo ""
echo "Run '${BINARY_NAME}' to start OpenBISS manually."
echo "Logs: ~/.openbiss/openbiss.log (macOS) or 'journalctl --user -u openbiss' (Linux systemd)"
