#!/bin/bash
# livesync-sync リリースパッケージ生成スクリプト
set -euo pipefail

cd "$(dirname "$0")/.."
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
OUTDIR="dist/livesync-sync-$VERSION"

echo "=== Building release $VERSION ==="
rm -rf dist/
mkdir -p "$OUTDIR"

# Linux
echo "--- Linux desktop ---"
CGO_ENABLED=1 go build -ldflags "-X main.version=$VERSION -s -w" -o "$OUTDIR/livesync" ./cmd/livesync

echo "--- Linux server ---"
CGO_ENABLED=0 go build -tags notray -ldflags "-X main.version=$VERSION -s -w" -o "$OUTDIR/livesync-server" ./cmd/livesync

# Windows
echo "--- Windows (CGO=OFF, recommended) ---"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags notray -trimpath -ldflags "-X main.version=$VERSION -s -w -buildid=" -o "$OUTDIR/livesync.exe" ./cmd/livesync

echo "--- Windows base64 (bypass SmartScreen) ---"
base64 "$OUTDIR/livesync.exe" > "$OUTDIR/livesync.b64"
cp scripts/decode.ps1 "$OUTDIR/decode.ps1"

# macOS (requires native build on macOS CI)
# echo "--- macOS ---"
# CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=$VERSION -s -w" -o "$OUTDIR/livesync-darwin" ./cmd/livesync

# Docs
cp README.md "$OUTDIR/"
cp config.example.json "$OUTDIR/"

# Summary
echo ""
echo "=== Release package: $OUTDIR ==="
ls -lh "$OUTDIR/"
echo ""
echo "Windows users:"
echo "  1. Download the zip"
echo "  2. Extract livesync.b64 and decode.ps1"
echo "  3. Run: powershell -ExecutionPolicy Bypass -File decode.ps1"
echo "  4. This produces livesync.exe (NOT marked by SmartScreen)"
echo "  5. If Defender still blocks, add exclusion:"
echo "     PowerShell(admin): Add-MpPreference -ExclusionPath \"C:\\path\\to\\folder\""
