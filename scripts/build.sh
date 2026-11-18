#!/usr/bin/env sh
# Builds ZVintedWatch from source: web interface, then a single binary.
#
#   ./scripts/build.sh                  # binary for this machine → ./bin/
#   GOOS=linux GOARCH=arm64 ./scripts/build.sh   # cross-compile
#
# Requirements: Go (see go.mod) and Node.js 20+.
set -eu

cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PKG="github.com/Nikola-Putnik/ZVintedWatch/internal/version"

echo "==> Building the web interface"
npm --prefix web ci --no-audit --no-fund
npm --prefix web run build

GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"
EXT=""
[ "$GOOS" = "windows" ] && EXT=".exe"
OUT="bin/zvintedwatch_${GOOS}_${GOARCH}${EXT}"

echo "==> Building $OUT (version $VERSION)"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath \
  -ldflags "-s -w -X $PKG.Version=$VERSION -X $PKG.Commit=$COMMIT -X $PKG.Date=$DATE" \
  -o "$OUT" ./cmd/zvintedwatch

echo "==> Done: $OUT"
