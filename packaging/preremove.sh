#!/bin/sh
set -e

# Only act on a real removal. On upgrade dpkg calls the old package's prerm
# with "upgrade" and rpm with "1"; stopping/disabling the service there would
# leave it disabled at boot after every update.
case "${1:-}" in
    upgrade|failed-upgrade|1|2) exit 0 ;;
esac

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
