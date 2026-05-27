#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BASE="${TMPDIR:-/tmp}/cfqt-e2e"
RUNNER="${ROOT_DIR}/scripts/e2e/run_trycloudflare_ab.sh"

chmod +x "$RUNNER"

for round in 1 2 3; do
  "$RUNNER" http2 "$round"
  "$RUNNER" quic "$round"
done

for proto in http2 quic; do
  echo "[${proto}]"
  for round in 1 2 3; do
    cat "$BASE/${proto}-round${round}/result.txt"
    echo
  done
done
