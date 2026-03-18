#!/usr/bin/env bash
set -euo pipefail

INSTALL_BIN="/usr/local/bin/picoclaw"
SERVICE_DST="/etc/systemd/system/picoclaw.service"
DATA_DIR="/var/lib/picoclaw"
SERVICE_USER="openclaw"
PURGE=false

# --- Parse flags ---
for arg in "$@"; do
    case "$arg" in
        --purge) PURGE=true ;;
        *)
            echo "Usage: $0 [--purge]" >&2
            exit 1
            ;;
    esac
done

# --- Root check ---
if [[ "$EUID" -ne 0 ]]; then
    echo "Error: This script must be run as root." >&2
    exit 1
fi

# --- Stop and disable service ---
if systemctl is-active --quiet picoclaw 2>/dev/null; then
    echo "Stopping picoclaw service..."
    systemctl stop picoclaw
fi
if systemctl is-enabled --quiet picoclaw 2>/dev/null; then
    echo "Disabling picoclaw service..."
    systemctl disable picoclaw
fi

# --- Remove service file ---
if [[ -f "$SERVICE_DST" ]]; then
    echo "Removing service file..."
    rm "$SERVICE_DST"
    systemctl daemon-reload
fi

# --- Remove binary ---
if [[ -f "$INSTALL_BIN" ]]; then
    echo "Removing binary..."
    rm "$INSTALL_BIN"
fi

# --- Purge data and user ---
if [[ "$PURGE" = true ]]; then
    if [[ -d "$DATA_DIR" ]]; then
        echo "Removing data directory $DATA_DIR..."
        rm -rf "$DATA_DIR"
    fi
    if id "$SERVICE_USER" &>/dev/null; then
        echo "Removing user '$SERVICE_USER'..."
        userdel "$SERVICE_USER"
    fi
    echo "PicoClaw fully purged."
else
    echo ""
    echo "PicoClaw service and binary removed."
    echo "Data preserved at $DATA_DIR. Run with --purge to remove data and user."
fi
