#!/bin/sh
set -e

# Set correct ownership
chown -R sendry:sendry /var/lib/sendry
chown -R sendry:sendry /var/log/sendry

# Create default config if not exists
if [ ! -f /etc/sendry/config.yaml ]; then
    if [ -f /etc/sendry/config.yaml.example ]; then
        cp /etc/sendry/config.yaml.example /etc/sendry/config.yaml
        chmod 640 /etc/sendry/config.yaml
        chown root:sendry /etc/sendry/config.yaml
        echo "Created default configuration at /etc/sendry/config.yaml"
        echo "Please edit this file before starting the service."
    fi
fi

# Reload systemd
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    echo ""
    echo "Sendry has been installed."
    echo ""
    echo "To start the service:"
    echo "  sudo systemctl start sendry"
    echo ""
    echo "To enable on boot:"
    echo "  sudo systemctl enable sendry"
    echo ""
    echo "Configuration file: /etc/sendry/config.yaml"
fi

exit 0
