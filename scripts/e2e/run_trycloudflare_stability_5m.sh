#!/usr/bin/env bash
set -euo pipefail

PROTO="${1:?proto required: http2 or quic}"
ROUND="${2:-1}"

case "$PROTO" in
  http2) PROTO_PORT_OFFSET=0 ;;
  quic) PROTO_PORT_OFFSET=100 ;;
  *)
    echo "unsupported proto: $PROTO (expected http2 or quic)" >&2
    exit 1
    ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

ROOT_DIR="$(cfqt_root_dir)"
BASE="${TMPDIR:-/tmp}/cfqt-e2e"
ORIGIN_DIR="$BASE/origin"
HTTP_PORT=$((28080 + PROTO_PORT_OFFSET + ROUND))
WS_PORT=$((20000 + PROTO_PORT_OFFSET + ROUND))
SOCKS_PORT=$((2080 + PROTO_PORT_OFFSET + ROUND))
CFQT_BIN="$BASE/cfqt"
SING_BIN="${SING_BOX_BIN:-/root/.local/bin/sing-box}"
UUID="${SING_BOX_UUID:-ff78bef5-223f-4845-8676-a2780c305ea4}"
OUT="$BASE/${PROTO}-stability5m-round${ROUND}"
PHASE_FILE="$OUT/phase"
SERVER_CONFIG="$OUT/sing-box-server.json"
CLIENT_CONFIG="$OUT/sing-box-client.json"
TEST_SECONDS=360
PROBE_INTERVAL_SECONDS=5
DOWNLOAD_EVERY_SECONDS=120

rm -rf "$OUT"
mkdir -p "$BASE" "$OUT" "$ORIGIN_DIR"
echo "startup" > "$PHASE_FILE"

cleanup() {
  set +e
  for p in "${PIDS[@]:-}"; do kill "$p" 2>/dev/null || true; done
}
trap cleanup EXIT
PIDS=()

if [[ ! -x "$SING_BIN" ]]; then
  echo "missing sing-box binary: $SING_BIN" >&2
  exit 1
fi

if [[ ! -x "$CFQT_BIN" ]]; then
  cfqt_build_binary "$ROOT_DIR" "$CFQT_BIN"
fi

truncate -s 512M "$ORIGIN_DIR/blob.bin"

cat > "$SERVER_CONFIG" <<JSON
{
  "log": {"level": "warn", "timestamp": true},
  "inbounds": [
    {
      "type": "vless",
      "tag": "vless-ws-in",
      "listen": "127.0.0.1",
      "listen_port": ${WS_PORT},
      "users": [{"uuid": "${UUID}"}],
      "transport": {"type": "ws", "path": "/ws"}
    }
  ],
  "outbounds": [{"type": "direct", "tag": "direct"}]
}
JSON

python3 -m http.server "$HTTP_PORT" --bind 127.0.0.1 --directory "$ORIGIN_DIR" >"$OUT/http-origin.log" 2>&1 &
PIDS+=($!)

"$SING_BIN" run -c "$SERVER_CONFIG" >"$OUT/sing-server.log" 2>&1 &
PIDS+=($!)

"$CFQT_BIN" \
  --cf-edge-protocol="$PROTO" \
  --cf-tunnel-target=ws://127.0.0.1:${WS_PORT} \
  --health-listen= \
  --log-level=info \
  >"$OUT/cfqt.log" 2>&1 &
CFQT_PID=$!
PIDS+=($CFQT_PID)

(
  while kill -0 "$CFQT_PID" 2>/dev/null; do
    rss="$(ps -o rss= -p "$CFQT_PID" | awk '{print $1}')"
    phase="$(cat "$PHASE_FILE" 2>/dev/null || echo startup)"
    if [[ -n "${rss:-}" ]]; then
      printf '%s %s %s\n' "$(date +%s)" "$phase" "$rss" >> "$OUT/rss.log"
    fi
    sleep 1
  done
) &
PIDS+=($!)

URL="$(cfqt_wait_url "$OUT/cfqt.log" "" 60 || true)"
if [[ "$URL" == "rate_limited" ]]; then
  echo "quick tunnel API rate limited" >&2
  exit 3
fi
if [[ -z "$URL" ]]; then
  echo "failed to obtain quick tunnel url" >&2
  exit 1
fi

HOST="${URL#https://}"
EDGE_IP="$(cfqt_wait_edge_ip "$HOST" 30 || true)"
if [[ -z "$EDGE_IP" ]]; then
  echo "failed to resolve quick tunnel hostname: $HOST" >&2
  exit 1
fi

cat > "$CLIENT_CONFIG" <<JSON
{
  "log": {"level": "warn", "timestamp": true},
  "inbounds": [
    {
      "type": "socks",
      "tag": "socks-in",
      "listen": "127.0.0.1",
      "listen_port": ${SOCKS_PORT}
    }
  ],
  "outbounds": [
    {
      "type": "vless",
      "tag": "vless-out",
      "server": "${EDGE_IP}",
      "server_port": 443,
      "uuid": "${UUID}",
      "tls": {"enabled": true, "server_name": "${HOST}"},
      "transport": {
        "type": "ws",
        "path": "/ws",
        "headers": {"Host": "${HOST}"}
      }
    }
  ]
}
JSON

"$SING_BIN" run -c "$CLIENT_CONFIG" >"$OUT/sing-client.log" 2>&1 &
PIDS+=($!)
echo "ready" > "$PHASE_FILE"

sleep 3
WARMED=0
for _ in $(seq 1 30); do
  if curl -fsS --socks5-hostname 127.0.0.1:${SOCKS_PORT} "http://127.0.0.1:${HTTP_PORT}/blob.bin" -r 0-1023 -o /dev/null; then
    WARMED=1
    break
  fi
  sleep 2
done
if [[ "$WARMED" -ne 1 ]]; then
  echo "warmup failed" >&2
  exit 1
fi
echo "warm" > "$PHASE_FILE"
sleep 1

START_TS="$(date +%s)"
END_TS=$((START_TS + TEST_SECONDS))
PROBE_TOTAL=0
PROBE_SUCCESS=0
PROBE_FAIL=0
PROBE_FAIL_DNS=0
PROBE_FAIL_HTTP=0
DOWNLOAD_TOTAL=0
DOWNLOAD_SUCCESS=0
DOWNLOAD_FAIL=0
DOWNLOAD_FAIL_DNS=0
DOWNLOAD_FAIL_HTTP=0
NEXT_DOWNLOAD_TS=$((START_TS + DOWNLOAD_EVERY_SECONDS))
echo "soak" > "$PHASE_FILE"

while [[ "$(date +%s)" -lt "$END_TS" ]]; do
  PROBE_TOTAL=$((PROBE_TOTAL + 1))
  if curl -fsS --max-time 20 --socks5-hostname 127.0.0.1:${SOCKS_PORT} \
    "http://127.0.0.1:${HTTP_PORT}/blob.bin" -r 0-1023 -o /dev/null; then
    PROBE_SUCCESS=$((PROBE_SUCCESS + 1))
  else
    PROBE_FAIL=$((PROBE_FAIL + 1))
    if cfqt_wait_edge_ip "$HOST" 1 >/dev/null 2>&1; then
      PROBE_FAIL_HTTP=$((PROBE_FAIL_HTTP + 1))
    else
      PROBE_FAIL_DNS=$((PROBE_FAIL_DNS + 1))
    fi
  fi

  if [[ "$(date +%s)" -ge "$NEXT_DOWNLOAD_TS" ]]; then
    DOWNLOAD_TOTAL=$((DOWNLOAD_TOTAL + 1))
    if curl -fsS --max-time 120 --socks5-hostname 127.0.0.1:${SOCKS_PORT} \
      "http://127.0.0.1:${HTTP_PORT}/blob.bin" -o /dev/null; then
      DOWNLOAD_SUCCESS=$((DOWNLOAD_SUCCESS + 1))
    else
      DOWNLOAD_FAIL=$((DOWNLOAD_FAIL + 1))
      if cfqt_wait_edge_ip "$HOST" 1 >/dev/null 2>&1; then
        DOWNLOAD_FAIL_HTTP=$((DOWNLOAD_FAIL_HTTP + 1))
      else
        DOWNLOAD_FAIL_DNS=$((DOWNLOAD_FAIL_DNS + 1))
      fi
    fi
    NEXT_DOWNLOAD_TS=$((NEXT_DOWNLOAD_TS + DOWNLOAD_EVERY_SECONDS))
  fi
  sleep "$PROBE_INTERVAL_SECONDS"
done

if [[ "$DOWNLOAD_TOTAL" -lt 3 ]]; then
  while [[ "$DOWNLOAD_TOTAL" -lt 3 ]]; do
    DOWNLOAD_TOTAL=$((DOWNLOAD_TOTAL + 1))
    if curl -fsS --max-time 120 --socks5-hostname 127.0.0.1:${SOCKS_PORT} \
      "http://127.0.0.1:${HTTP_PORT}/blob.bin" -o /dev/null; then
      DOWNLOAD_SUCCESS=$((DOWNLOAD_SUCCESS + 1))
    else
      DOWNLOAD_FAIL=$((DOWNLOAD_FAIL + 1))
      if cfqt_wait_edge_ip "$HOST" 1 >/dev/null 2>&1; then
        DOWNLOAD_FAIL_HTTP=$((DOWNLOAD_FAIL_HTTP + 1))
      else
        DOWNLOAD_FAIL_DNS=$((DOWNLOAD_FAIL_DNS + 1))
      fi
    fi
  done
fi

echo "final" > "$PHASE_FILE"
sleep 1
NOW_TS="$(date +%s)"
DURATION=$((NOW_TS - START_TS))
if [[ "$DURATION" -le 0 ]]; then DURATION=1; fi

RSS_READY_KB=$(awk '$2=="ready" {v=$3} END{print v+0}' "$OUT/rss.log")
RSS_WARM_KB=$(awk '$2=="warm" {v=$3} END{print v+0}' "$OUT/rss.log")
PEAK_RSS_KB=$(awk 'BEGIN{m=0} $2=="soak" {if ($3>m) m=$3} END{print m+0}' "$OUT/rss.log")
RSS_FINAL_KB=$(awk '$2=="final" {v=$3} END{print v+0}' "$OUT/rss.log")
PROBE_FAIL_RATE=$(awk -v s="$PROBE_SUCCESS" -v f="$PROBE_FAIL" 'BEGIN{n=s+f; if(n==0){printf "0.00"}else{printf "%.2f", (f*100)/n}}')

cat > "$OUT/result.txt" <<TXT
proto=${PROTO}
round=${ROUND}
url=${URL}
test_seconds=${DURATION}
probe_interval_seconds=${PROBE_INTERVAL_SECONDS}
probes_total=${PROBE_TOTAL}
probes_success=${PROBE_SUCCESS}
probes_fail=${PROBE_FAIL}
probes_fail_dns=${PROBE_FAIL_DNS}
probes_fail_http=${PROBE_FAIL_HTTP}
probe_fail_rate_percent=${PROBE_FAIL_RATE}
download_interval_seconds=${DOWNLOAD_EVERY_SECONDS}
downloads_total=${DOWNLOAD_TOTAL}
downloads_success=${DOWNLOAD_SUCCESS}
downloads_fail=${DOWNLOAD_FAIL}
downloads_fail_dns=${DOWNLOAD_FAIL_DNS}
downloads_fail_http=${DOWNLOAD_FAIL_HTTP}
rss_ready_kb=${RSS_READY_KB}
rss_warm_kb=${RSS_WARM_KB}
peak_rss_kb=${PEAK_RSS_KB}
rss_final_kb=${RSS_FINAL_KB}
TXT

cat "$OUT/result.txt"
