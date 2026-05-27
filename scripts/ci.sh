#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(tr -d '\n' < "${ROOT_DIR}/VERSION")"
IMAGE_TAG="cf-quicktunnel-ipv6pool:${VERSION}"

cd "${ROOT_DIR}"

echo "[1/3] Running tests"
go test ./...

echo "[2/3] Building release binary"
"${ROOT_DIR}/scripts/build-release.sh"

echo "[3/3] Building Docker image ${IMAGE_TAG}"
docker build -t "${IMAGE_TAG}" "${ROOT_DIR}"

echo "CI pipeline completed"
