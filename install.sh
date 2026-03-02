#!/bin/bash

set -e

INSTALL_TYPE="client"

while [[ $# -gt 0 ]]; do
    case $1 in
        --server) INSTALL_TYPE="server"; shift ;;
        --help|-h)
            echo "Usage: $0 [--server]"
            echo "  --server    Install ogrok server instead of client"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

die()  { echo -e "${RED}error: $1${NC}" >&2; exit 1; }
info() { echo -e "${GREEN}$1${NC}"; }
warn() { echo -e "${YELLOW}$1${NC}"; }

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case $OS in
        linux*)  OS="linux" ;;
        darwin*) OS="darwin" ;;
        *) die "Unsupported OS: $OS" ;;
    esac

    case $ARCH in
        x86_64|amd64) ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *) die "Unsupported architecture: $ARCH" ;;
    esac

    echo "Platform: ${OS}-${ARCH}"
}

get_install_dir() {
    if [[ $EUID -eq 0 ]] || [[ -w "/usr/local/bin" ]]; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="$HOME/.local/bin"
        mkdir -p "$INSTALL_DIR" 2>/dev/null || die "Cannot create $INSTALL_DIR"

        if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
            warn "$INSTALL_DIR is not in your PATH"
            echo "    export PATH=\"\$PATH:$INSTALL_DIR\""
        fi
    fi
}

install_ogrok() {
    command -v curl >/dev/null 2>&1 || die "curl is required"

    if [[ "$INSTALL_TYPE" == "server" ]]; then
        BINARY_NAME="ogrok-server-${OS}-${ARCH}"
        INSTALL_NAME="ogrok-server"
    else
        BINARY_NAME="ogrok-${OS}-${ARCH}"
        INSTALL_NAME="ogrok"
    fi

    DOWNLOAD_URL="https://github.com/abdullahob/ogrok/releases/latest/download/${BINARY_NAME}"
    TEMP_FILE=$(mktemp)

    echo "Downloading ${INSTALL_NAME}..."
    curl -fsSL "$DOWNLOAD_URL" -o "$TEMP_FILE" || { rm -f "$TEMP_FILE"; die "Download failed"; }

    [[ -s "$TEMP_FILE" ]] || { rm -f "$TEMP_FILE"; die "Downloaded file is empty"; }

    mv "$TEMP_FILE" "$INSTALL_DIR/$INSTALL_NAME"
    chmod +x "$INSTALL_DIR/$INSTALL_NAME"

    info "${INSTALL_NAME} installed to ${INSTALL_DIR}"
}

verify_installation() {
    if command -v "$INSTALL_NAME" >/dev/null 2>&1; then
        VERSION=$($INSTALL_NAME --version 2>/dev/null || echo "unknown")
        info "${INSTALL_NAME} ready (${VERSION})"
    elif [[ "$INSTALL_DIR" == "$HOME/.local/bin" ]]; then
        warn "Installed but not in PATH. Run: export PATH=\"\$PATH:$INSTALL_DIR\""
    fi
}

main() {
    echo "Installing ogrok ${INSTALL_TYPE}..."
    detect_platform
    get_install_dir
    install_ogrok
    verify_installation

    echo
    if [[ "$INSTALL_TYPE" == "server" ]]; then
        echo "Quick start:"
        echo "  ogrok-server --config /path/to/server.yaml"
    else
        echo "Quick start:"
        echo "  export OGROK_TOKEN=your-token"
        echo "  ogrok http 3000"
    fi
}

trap 'die "Installation failed"' ERR
main "$@"
