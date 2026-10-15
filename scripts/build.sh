#!/bin/bash
# Build SmartDisk: frontend -> Go binary (linux/amd64) -> bundle -> fnpack .fpk
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEB="$ROOT/web"
SERVER="$ROOT/server"
APP="$ROOT/SmartDisk"
GO="${GO:-go}"

# Single source of truth for the version: SmartDisk/manifest.
VERSION="$(sed -n 's/^version[[:space:]]*=[[:space:]]*//p' "$APP/manifest" | tr -d '[:space:]')"
if [ -z "$VERSION" ]; then
  echo "!! could not read version from $APP/manifest" >&2
  exit 1
fi
echo "==> Version: $VERSION"

PACKAGE_VERSION="$(sed -n 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$WEB/package.json" | head -n 1)"
if [ "$PACKAGE_VERSION" != "$VERSION" ]; then
  echo "!! version mismatch: manifest=$VERSION web/package.json=$PACKAGE_VERSION" >&2
  exit 1
fi

echo "==> [1/6] Check frontend types"
(cd "$WEB" && npm run typecheck)

echo "==> [2/6] Build frontend"
(cd "$WEB" && npm run build)

echo "==> [3/6] Cross-compile Go backend (linux/amd64, static)"
mkdir -p "$APP/app/server"
(cd "$SERVER" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build \
  -trimpath -ldflags="-s -w -extldflags=-static -X main.Version=$VERSION" \
  -o "$APP/app/server/smartdisk" .)

echo "==> [4/6] Replace staged web assets"
# The binary embeds the frontend; keep the inspectable www copy identical.
rm -rf "$APP/app/www"
mkdir -p "$APP/app/www"
cp -R "$SERVER/web/dist/." "$APP/app/www/"

echo "==> [5/6] Stage bundled smartctl/nvme (linux x86_64)"
# Replace only these optional tools; preserve the smartdisk-cli wrapper.
mkdir -p "$APP/app/bin"
for tool in smartctl nvme; do
  rm -f "$APP/app/bin/$tool"
  if [ -f "$ROOT/tools/bin/$tool" ]; then
    install -m 0755 "$ROOT/tools/bin/$tool" "$APP/app/bin/$tool"
  fi
done

echo "==> [6/6] Package .fpk"
(cd "$APP" && fnpack build)

echo
echo "Done. Artifacts:"
ls -1 "$APP"/*.fpk 2>/dev/null || echo "  (no .fpk produced)"
