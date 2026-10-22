#!/bin/bash
# Run the backend in mock mode (canned SMART data) for local UI development.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO="${GO:-go}"

export SMARTDISK_MODE=all
export SMARTDISK_PORT="${SMARTDISK_PORT:-18315}"
export SMARTDISK_MOCK_DIR="${SMARTDISK_MOCK_DIR:-$ROOT/server/mock}"
export SMARTDISK_WEB_ROOT="${SMARTDISK_WEB_ROOT:-$ROOT/server/web/dist}"

echo "SmartDisk dev backend on http://localhost:${SMARTDISK_PORT}"
echo "Mock dir: ${SMARTDISK_MOCK_DIR}"
cd "$ROOT/server" && exec "$GO" run .
