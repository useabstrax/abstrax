#!/bin/sh
set -e

# Create required directories if they don't exist.
mkdir -p /etc/abstrax/projects
mkdir -p /var/lib/abstrax
mkdir -p /var/log/abstrax

# Set secure permissions.
chmod 750 /etc/abstrax
chmod 750 /var/lib/abstrax
chmod 750 /var/log/abstrax

# Do NOT enable or start the agent service - it is not yet implemented.
# systemctl enable abstrax-agent
# systemctl start abstrax-agent

echo "Abstrax installed successfully."
echo "Run 'abstrax doctor' to inspect your system."
echo "Run 'abstrax --help' for available commands."
