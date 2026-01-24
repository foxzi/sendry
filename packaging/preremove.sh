#!/bin/sh
set -e

# Stop and disable service if running
if command -v systemctl >/dev/null 2>&1; then
    if systemctl is-active --quiet sendry; then
        echo "Stopping sendry service..."
        systemctl stop sendry
    fi
    if systemctl is-enabled --quiet sendry 2>/dev/null; then
        echo "Disabling sendry service..."
        systemctl disable sendry
    fi
fi

exit 0
