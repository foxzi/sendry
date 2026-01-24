#!/bin/sh
set -e

# Create sendry user and group if they don't exist
if ! getent group sendry >/dev/null 2>&1; then
    groupadd --system sendry
fi

if ! getent passwd sendry >/dev/null 2>&1; then
    useradd --system \
        --gid sendry \
        --home-dir /var/lib/sendry \
        --shell /sbin/nologin \
        --comment "Sendry MTA" \
        sendry
fi

exit 0
