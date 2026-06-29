#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
PREFIX="${PREFIX:-$HOME/.local}"
BIN_DIR="$PREFIX/bin"

mkdir -p "$BIN_DIR"
"$GO_BIN" build -o "$BIN_DIR/toolcapsule" ./cmd/toolcapsule

echo "installed $BIN_DIR/toolcapsule"
echo "make sure $BIN_DIR is on your PATH"
