#!/bin/bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

die()  { echo -e "${RED}error: $1${NC}" >&2; exit 1; }
info() { echo -e "${GREEN}$1${NC}"; }
warn() { echo -e "${YELLOW}$1${NC}"; }

cd "$(dirname "$0")"

command -v docker >/dev/null 2>&1 || die "Docker is not installed"
docker compose version >/dev/null 2>&1 || command -v docker-compose >/dev/null 2>&1 || die "Docker Compose is not installed"

mkdir -p ./data/certs

if grep -q "your-secure-token-here" ./configs/server.yaml 2>/dev/null; then
    if command -v openssl >/dev/null 2>&1; then
        AUTH_TOKEN=$(openssl rand -hex 32)
    else
        AUTH_TOKEN=$(head -c 32 /dev/urandom | xxd -p -c 32)
    fi
    sed -i.bak "s/your-secure-token-here/$AUTH_TOKEN/" ./configs/server.yaml
    info "Generated auth token"
fi

if grep -q "tunnel.yourdomain.com" ./configs/server.yaml 2>/dev/null; then
    warn "Update base_domain in ./configs/server.yaml"
fi

echo "Building and deploying..."
docker-compose up --build -d

sleep 10

if docker-compose ps | grep -q "Up"; then
    info "ogrok server is running"
else
    die "Failed to start. Check: docker-compose logs"
fi

if command -v ufw >/dev/null 2>&1; then
    sudo ufw allow 80/tcp 2>/dev/null || true
    sudo ufw allow 443/tcp 2>/dev/null || true
fi

echo
echo "========================================="
[[ -n "${AUTH_TOKEN:-}" ]] && echo "Auth token: $AUTH_TOKEN"
echo "========================================="
echo
echo "Management:"
echo "  Logs:    docker-compose logs -f"
echo "  Restart: docker-compose restart"
echo "  Stop:    docker-compose down"
echo "  Health:  curl http://localhost:8080/health"
