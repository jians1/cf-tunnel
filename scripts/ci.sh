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

if [[ "${CI_E2E_MULTI:-0}" == "1" ]]; then
  echo "[optional] Running multi-tunnel real-link e2e (http2/quic)"
  "${ROOT_DIR}/scripts/e2e/run_trycloudflare_multi_tunnel_real.sh" http2 1
  "${ROOT_DIR}/scripts/e2e/run_trycloudflare_multi_tunnel_real.sh" quic 1
fi

echo "CI pipeline completed"
