#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(tr -d '\n' < "${ROOT_DIR}/VERSION")"
OUT_DIR="${ROOT_DIR}/dist"

GOOS_VALUE="${GOOS:-linux}"
GOARCH_VALUE="${GOARCH:-amd64}"
BIN_NAME="cf-quicktunnel-ipv6pool"
OUT_PATH="${OUT_DIR}/${BIN_NAME}-${VERSION}-${GOOS_VALUE}-${GOARCH_VALUE}"
SHA_PATH="${OUT_PATH}.sha256"
MANIFEST_PATH="${OUT_DIR}/${BIN_NAME}-${VERSION}-${GOOS_VALUE}-${GOARCH_VALUE}.manifest.txt"

mkdir -p "${OUT_DIR}"

cd "${ROOT_DIR}"

echo "Building ${OUT_PATH}"
CGO_ENABLED=0 GOOS="${GOOS_VALUE}" GOARCH="${GOARCH_VALUE}" \
  go build -buildvcs=false -trimpath -ldflags="-s -w" \
  -o "${OUT_PATH}" ./cmd/app

sha256sum "${OUT_PATH}" | sed "s#${OUT_PATH}#$(basename "${OUT_PATH}")#" > "${SHA_PATH}"

cat > "${MANIFEST_PATH}" <<EOF
name=${BIN_NAME}
version=${VERSION}
goos=${GOOS_VALUE}
goarch=${GOARCH_VALUE}
binary=$(basename "${OUT_PATH}")
sha256_file=$(basename "${SHA_PATH}")
EOF

echo "Done: ${OUT_PATH}"
echo "Done: ${SHA_PATH}"
echo "Done: ${MANIFEST_PATH}"
