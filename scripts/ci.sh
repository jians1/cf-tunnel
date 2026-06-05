#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${ROOT_DIR}"

echo "[1/2] Running tests"
go test ./...

echo "[2/2] Building release binary"
"${ROOT_DIR}/scripts/build-release.sh"

echo "CI pipeline completed"
