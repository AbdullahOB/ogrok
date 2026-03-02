#!/bin/bash

set -e

mkdir -p bin

LDFLAGS="-s -w"

echo "Building release binaries..."

GOOS=linux  GOARCH=amd64 go build -ldflags="$LDFLAGS" -o bin/ogrok-linux-amd64  ./cmd/client
GOOS=linux  GOARCH=arm64 go build -ldflags="$LDFLAGS" -o bin/ogrok-linux-arm64  ./cmd/client
GOOS=darwin GOARCH=amd64 go build -ldflags="$LDFLAGS" -o bin/ogrok-darwin-amd64 ./cmd/client
GOOS=darwin GOARCH=arm64 go build -ldflags="$LDFLAGS" -o bin/ogrok-darwin-arm64 ./cmd/client

echo "Done."
ls -lh bin/ogrok-*
