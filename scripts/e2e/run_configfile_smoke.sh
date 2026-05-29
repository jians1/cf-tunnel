#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

ROOT_DIR="$(cfqt_root_dir)"
BASE="${TMPDIR:-/tmp}/cfqt-config-smoke"
OUT="$BASE/out"
BIN="$BASE/cfqt"

rm -rf "$BASE"
mkdir -p "$OUT/o1" "$OUT/o2"
echo "origin-one-ok" > "$OUT/o1/index.html"
echo "origin-two-ok" > "$OUT/o2/index.html"

cleanup() {
  set +e
  for p in "${PIDS[@]:-}"; do kill "$p" 2>/dev/null || true; done
}
trap cleanup EXIT
PIDS=()

python3 -m http.server 18081 --bind 127.0.0.1 --directory "$OUT/o1" >"$OUT/o1.log" 2>&1 &
PIDS+=($!)
python3 -m http.server 18082 --bind 127.0.0.1 --directory "$OUT/o2" >"$OUT/o2.log" 2>&1 &
PIDS+=($!)

cfqt_build_binary "$ROOT_DIR" "$BIN"

cat > "$OUT/single.json" <<'JSON'
{
  "log_format": "json",
  "health_listen": "",
  "cf_tunnel": {
    "QuickService": "https://api.trycloudflare.com",
    "EdgeProtocol": "quic",
    "Target": "http://127.0.0.1:18081"
  }
}
JSON

cat > "$OUT/multi.json" <<'JSON'
{
  "log_format": "json",
  "health_listen": "",
  "tunnels": [
    {
      "name": "alpha",
      "CFTunnel": {
        "QuickService": "https://api.trycloudflare.com",
        "EdgeProtocol": "quic",
        "Target": "http://127.0.0.1:18081"
      }
    },
    {
      "name": "beta",
      "CFTunnel": {
        "QuickService": "https://api.trycloudflare.com",
        "EdgeProtocol": "http2",
        "Target": "http://127.0.0.1:18082"
      }
    }
  ]
}
JSON

# single
"$BIN" --config="$OUT/single.json" >"$OUT/single-app.log" 2>&1 &
APP1=$!
PIDS+=($APP1)

SINGLE_URL="$(cfqt_wait_url "$OUT/single-app.log" "" 80 || true)"
if [[ "$SINGLE_URL" == "rate_limited" ]]; then
  echo "single_status=rate_limited"
  tail -n 80 "$OUT/single-app.log" || true
  exit 3
fi
if [[ -z "$SINGLE_URL" ]]; then
  echo "single_status=failed_no_url"
  tail -n 80 "$OUT/single-app.log" || true
  exit 1
fi
SINGLE_RESULT="$(cfqt_check_http_body "single" "$SINGLE_URL" "origin-one-ok" || true)"
kill "$APP1" 2>/dev/null || true
wait "$APP1" 2>/dev/null || true

# multi
"$BIN" --config="$OUT/multi.json" >"$OUT/multi-app.log" 2>&1 &
APP2=$!
PIDS+=($APP2)

ALPHA_URL=""
BETA_URL=""
for _ in $(seq 1 90); do
  ALPHA_URL="$(cfqt_extract_url "$OUT/multi-app.log" '"tunnel_name":"alpha"' || true)"
  BETA_URL="$(cfqt_extract_url "$OUT/multi-app.log" '"tunnel_name":"beta"' || true)"
  if [[ -n "$ALPHA_URL" && -n "$BETA_URL" ]]; then
    break
  fi
  if cfqt_detect_rate_limit "$OUT/multi-app.log"; then
    ALPHA_URL="rate_limited"
    BETA_URL="rate_limited"
    break
  fi
  sleep 1
done
if [[ "$ALPHA_URL" == "rate_limited" || "$BETA_URL" == "rate_limited" ]]; then
  echo "multi_status=rate_limited"
  tail -n 120 "$OUT/multi-app.log" || true
  exit 3
fi
if [[ -z "$ALPHA_URL" || -z "$BETA_URL" ]]; then
  echo "multi_status=failed_no_url"
  tail -n 120 "$OUT/multi-app.log" || true
  exit 1
fi
ALPHA_RESULT="$(cfqt_check_http_body "alpha" "$ALPHA_URL" "origin-one-ok" || true)"
BETA_RESULT="$(cfqt_check_http_body "beta" "$BETA_URL" "origin-two-ok" || true)"
kill "$APP2" 2>/dev/null || true
wait "$APP2" 2>/dev/null || true

SINGLE_OK="$(printf '%s\n' "$SINGLE_RESULT" | sed -n 's/^single_ok=//p')"
ALPHA_OK="$(printf '%s\n' "$ALPHA_RESULT" | sed -n 's/^alpha_ok=//p')"
BETA_OK="$(printf '%s\n' "$BETA_RESULT" | sed -n 's/^beta_ok=//p')"

cat <<TXT
single_url=${SINGLE_URL}
${SINGLE_RESULT}
alpha_url=${ALPHA_URL}
${ALPHA_RESULT}
beta_url=${BETA_URL}
${BETA_RESULT}
TXT

if [[ "$SINGLE_OK" -ne 1 || "$ALPHA_OK" -ne 1 || "$BETA_OK" -ne 1 ]]; then
  echo "smoke_result=failed"
  exit 2
fi
echo "smoke_result=pass"
