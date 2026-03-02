#!/bin/bash

# Server setup for Ubuntu/Debian systems

set -euo pipefail

OGROK_USER="ogrok"
INSTALL_DIR="/opt/ogrok"
SERVICE_NAME="ogrok-server"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

die()  { echo -e "${RED}error: $1${NC}" >&2; exit 1; }
info() { echo -e "${GREEN}$1${NC}"; }
warn() { echo -e "${YELLOW}$1${NC}"; }

[[ $EUID -ne 0 ]] && die "This script must be run as root"

if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    [[ "$ID" != "ubuntu" && "$ID" != "debian" ]] && die "Only Ubuntu/Debian supported"
else
    die "Cannot detect OS"
fi

echo "Setting up ogrok server on ${ID} ${VERSION_ID}..."

apt-get update -qq
apt-get install -y -qq wget curl ufw >/dev/null

if ! id "$OGROK_USER" &>/dev/null; then
    useradd --system --shell /bin/false --home-dir "$INSTALL_DIR" --create-home "$OGROK_USER"
fi

mkdir -p "$INSTALL_DIR"/{bin,configs,certs}
chown -R "$OGROK_USER:$OGROK_USER" "$INSTALL_DIR"
chmod 755 "$INSTALL_DIR"
chmod 750 "$INSTALL_DIR"/{configs,certs}

[[ -f "./bin/ogrok-server" ]] || die "Binary not found. Run 'make build' first."

cp "./bin/ogrok-server" "$INSTALL_DIR/bin/"
chmod 755 "$INSTALL_DIR/bin/ogrok-server"

[[ -f "./configs/server.yaml" ]] || die "Config file not found"
cp "./configs/server.yaml" "$INSTALL_DIR/configs/"

AUTH_TOKEN=$(openssl rand -hex 32)
sed -i "s/token-1-xxxx/$AUTH_TOKEN/" "$INSTALL_DIR/configs/server.yaml"
sed -i '/token-2-yyyy/d' "$INSTALL_DIR/configs/server.yaml"
sed -i '/dev-token-123/d' "$INSTALL_DIR/configs/server.yaml"

chown -R "$OGROK_USER:$OGROK_USER" "$INSTALL_DIR"

[[ -f "./deploy/ogrok-server.service" ]] || die "Service file not found"
cp "./deploy/ogrok-server.service" "/etc/systemd/system/"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

ufw --force enable
ufw allow ssh
ufw allow 80/tcp
ufw allow 443/tcp

systemctl start "$SERVICE_NAME"
sleep 2

if systemctl is-active --quiet "$SERVICE_NAME"; then
    info "ogrok server started"
else
    die "Failed to start. Check: journalctl -u $SERVICE_NAME -f"
fi

echo
echo "========================================="
echo "Auth token: $AUTH_TOKEN"
echo "========================================="
echo
echo "Next steps:"
echo "  1. Set up DNS (see deploy/dns-setup.md)"
echo "  2. Update base_domain in $INSTALL_DIR/configs/server.yaml"
echo "  3. Restart: sudo systemctl restart $SERVICE_NAME"
echo
echo "Client usage:"
echo "  ogrok http 3000 --server your-domain.com --token $AUTH_TOKEN"
