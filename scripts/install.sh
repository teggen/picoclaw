#!/usr/bin/env bash
set -euo pipefail

BINARY_SRC="${1:-./build/picoclaw}"
INSTALL_BIN="/usr/local/bin/picoclaw"
SERVICE_SRC="$(dirname "$0")/picoclaw.service"
SERVICE_DST="/etc/systemd/system/picoclaw.service"
DATA_DIR="/var/lib/picoclaw"
SERVICE_USER="openclaw"

# --- Root check ---
if [[ "$EUID" -ne 0 ]]; then
    echo "Error: This script must be run as root." >&2
    exit 1
fi

# --- Resolve and verify binary ---
if [[ ! -f "$BINARY_SRC" ]]; then
    echo "Error: Binary not found at '$BINARY_SRC'." >&2
    echo "Build first with 'make build' or pass the binary path as an argument." >&2
    exit 1
fi
if [[ ! -x "$BINARY_SRC" ]]; then
    echo "Error: '$BINARY_SRC' is not executable." >&2
    exit 1
fi

# --- Create system user ---
if ! id "$SERVICE_USER" &>/dev/null; then
    echo "Creating system user '$SERVICE_USER'..."
    useradd --system --home-dir "$DATA_DIR" \
        --shell /usr/sbin/nologin --comment "PicoClaw service account" "$SERVICE_USER"
else
    echo "User '$SERVICE_USER' already exists."
fi

# --- Install binary (atomic: tmp + mv) ---
echo "Installing binary to $INSTALL_BIN..."
TMPBIN="$(mktemp "${INSTALL_BIN}.XXXXXX")"
cp "$BINARY_SRC" "$TMPBIN"
chown root:root "$TMPBIN"
chmod 0755 "$TMPBIN"
mv "$TMPBIN" "$INSTALL_BIN"

# --- Create data directory ---
echo "Setting up data directory at $DATA_DIR..."
mkdir -p "$DATA_DIR"
chown "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR"
chmod 0750 "$DATA_DIR"

# --- Run onboard if no config exists ---
CONFIG_FILE="$DATA_DIR/config.json"
if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "Running initial onboard..."
    sudo -u "$SERVICE_USER" \
        PICOCLAW_HOME="$DATA_DIR" HOME="$DATA_DIR" \
        "$INSTALL_BIN" onboard
else
    echo "Config already exists at $CONFIG_FILE, skipping onboard."
fi

# --- Install systemd service ---
if [[ ! -f "$SERVICE_SRC" ]]; then
    echo "Error: Service file not found at '$SERVICE_SRC'." >&2
    exit 1
fi
echo "Installing systemd service..."
cp "$SERVICE_SRC" "$SERVICE_DST"
chmod 0644 "$SERVICE_DST"
systemctl daemon-reload
systemctl enable picoclaw

echo ""
echo "=== PicoClaw installed successfully ==="
echo ""
echo "Next steps:"
echo "  1. Edit the config:    sudo -u $SERVICE_USER editor $CONFIG_FILE"
echo "  2. Start the service:  sudo systemctl start picoclaw"
echo "  3. View logs:          journalctl -u picoclaw -f"
