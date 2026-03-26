#!/usr/bin/env bash
set -euo pipefail

BINARY_SRC="${1:-./build/picoclaw}"
FOLLOW_MODE=false
if [[ "${2:-}" == "--follow" ]]; then
    FOLLOW_MODE=true
fi
INSTALL_BIN="/usr/local/bin/picoclaw"

# Derive CLI binary path from the same build directory.
BUILD_DIR="$(dirname "$BINARY_SRC")"
CLI_SRC="$BUILD_DIR/picoclaw-cli"
CLI_INSTALL_BIN="/usr/local/bin/picoclaw-cli"

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

# --- Replace CLI binary if present ---
if [[ -f "$CLI_SRC" && -x "$CLI_SRC" ]]; then
    echo "Updating CLI binary at $CLI_INSTALL_BIN..."
    TMPCLIIN="$(mktemp "${CLI_INSTALL_BIN}.XXXXXX")"
    cp "$CLI_SRC" "$TMPCLIIN"
    chown root:root "$TMPCLIIN"
    chmod 0755 "$TMPCLIIN"
    mv "$TMPCLIIN" "$CLI_INSTALL_BIN"
else
    echo "Note: CLI binary not found at '$CLI_SRC'; skipping CLI update."
fi

# --- Restart service if running ---
if systemctl is-active --quiet picoclaw 2>/dev/null; then
    RESTART_TIME="$(date '+%Y-%m-%d %H:%M:%S')"
    echo "Restarting picoclaw service..."
    systemctl restart picoclaw
else
    echo "Note: picoclaw service is not currently active; skipping restart."
    RESTART_TIME=""
fi

echo ""
echo "=== PicoClaw updated successfully ==="
echo ""

# --- Post-update health check ---
echo "Service status:"
systemctl status picoclaw --no-pager || true
echo ""

if [[ -n "$RESTART_TIME" ]]; then
    if [[ "$FOLLOW_MODE" == true ]]; then
        echo "Following logs since restart (Ctrl+C to stop):"
        journalctl -u picoclaw --since="$RESTART_TIME" -f --no-pager || true
    else
        echo "Streaming logs for 10 seconds since restart:"
        timeout 10 journalctl -u picoclaw --since="$RESTART_TIME" -f --no-pager || true
    fi
else
    echo "Recent logs:"
    journalctl -u picoclaw -n 20 --no-pager || true
fi
