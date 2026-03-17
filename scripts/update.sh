#!/usr/bin/env bash
set -euo pipefail

BINARY_SRC="${1:-./build/picoclaw}"
INSTALL_BIN="/usr/local/bin/picoclaw"

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

# --- Verify existing installation ---
if [[ ! -f "$INSTALL_BIN" ]]; then
    echo "Error: No existing installation found at '$INSTALL_BIN'." >&2
    echo "Run install.sh first." >&2
    exit 1
fi

# --- Replace binary (atomic: tmp + mv) ---
echo "Updating binary at $INSTALL_BIN..."
TMPBIN="$(mktemp "${INSTALL_BIN}.XXXXXX")"
cp "$BINARY_SRC" "$TMPBIN"
chown root:root "$TMPBIN"
chmod 0755 "$TMPBIN"
mv "$TMPBIN" "$INSTALL_BIN"

# --- Restart service if running ---
if systemctl is-active --quiet picoclaw 2>/dev/null; then
    echo "Restarting picoclaw service..."
    systemctl restart picoclaw
fi

echo ""
echo "=== PicoClaw updated successfully ==="
echo ""
echo "  View logs: journalctl -u picoclaw -f"
