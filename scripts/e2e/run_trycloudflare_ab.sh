#!/usr/bin/env bash
set -euo pipefail

PROTO="${1:?proto required: http2 or quic}"
ROUND="${2:?round required}"

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
API_DIR="$BASE/api-origin"
HTTP_PORT=$((18080 + PROTO_PORT_OFFSET + ROUND))
API_PORT=$((19080 + PROTO_PORT_OFFSET + ROUND))
WS_PORT=$((10000 + PROTO_PORT_OFFSET + ROUND))
SOCKS_PORT=$((1080 + PROTO_PORT_OFFSET + ROUND))
CFQT_BIN="$BASE/cfqt"
SING_BIN="${SING_BOX_BIN:-/root/.local/bin/sing-box}"
UUID="${SING_BOX_UUID:-ff78bef5-223f-4845-8676-a2780c305ea4}"
OUT="$BASE/${PROTO}-round${ROUND}"
PHASE_FILE="$OUT/phase"
SERVER_CONFIG="$OUT/sing-box-server.json"

rm -rf "$OUT"
mkdir -p "$BASE" "$OUT" "$ORIGIN_DIR" "$API_DIR"
mkdir -p "$API_DIR/api"
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

cfqt_build_binary "$ROOT_DIR" "$CFQT_BIN"

truncate -s 1G "$ORIGIN_DIR/blob.bin"
echo "default-ok" > "$ORIGIN_DIR/default.txt"
echo "api-ok" > "$API_DIR/api/ping.txt"

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

python3 -m http.server "$API_PORT" --bind 127.0.0.1 --directory "$API_DIR" >"$OUT/api-origin.log" 2>&1 &
PIDS+=($!)

"$SING_BIN" run -c "$SERVER_CONFIG" >"$OUT/sing-server.log" 2>&1 &
PIDS+=($!)

"$CFQT_BIN" \
  --cf-edge-protocol="$PROTO" \
  --cf-tunnel-target=http://127.0.0.1:${HTTP_PORT} \
  --cf-route=/ws=ws://127.0.0.1:${WS_PORT} \
  --cf-route=/api/*=http://127.0.0.1:${API_PORT} \
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

cat > "$OUT/sing-box-client.json" <<JSON
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

"$SING_BIN" run -c "$OUT/sing-box-client.json" >"$OUT/sing-client.log" 2>&1 &
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

API_BODY="$(cfqt_curl_https_resolved_retry "${URL}/api/ping.txt" "$EDGE_IP" 30 2 || true)"
if [[ "$API_BODY" != "api-ok" ]]; then
  echo "path-routing check failed: /api/* did not hit api backend" >&2
  exit 1
fi
DEFAULT_BODY="$(cfqt_curl_https_resolved_retry "${URL}/default.txt" "$EDGE_IP" 30 2 || true)"
if [[ "$DEFAULT_BODY" != "default-ok" ]]; then
  echo "path-routing check failed: default route did not hit default backend" >&2
  exit 1
fi
PATH_ROUTING_CHECK="pass"

echo "warm" > "$PHASE_FILE"
sleep 1

START=$(date +%s)
echo "download" > "$PHASE_FILE"
curl -fsS --socks5-hostname 127.0.0.1:${SOCKS_PORT} "http://127.0.0.1:${HTTP_PORT}/blob.bin" -o "$OUT/blob.bin"
END=$(date +%s)
echo "final" > "$PHASE_FILE"
sleep 1

SHA=$(sha256sum "$OUT/blob.bin" | awk '{print $1}')
RSS_READY_KB=$(awk '$2=="ready" {v=$3} END{print v+0}' "$OUT/rss.log")
RSS_WARM_KB=$(awk '$2=="warm" {v=$3} END{print v+0}' "$OUT/rss.log")
PEAK_RSS_KB=$(awk 'BEGIN{m=0} $2=="download" {if ($3>m) m=$3} END{print m+0}' "$OUT/rss.log")
RSS_FINAL_KB=$(awk '$2=="final" {v=$3} END{print v+0}' "$OUT/rss.log")
SIZE=$(stat -c %s "$OUT/blob.bin")
DUR=$((END-START))
if [[ "$DUR" -le 0 ]]; then DUR=1; fi
MBPS=$(awk -v s="$SIZE" -v d="$DUR" 'BEGIN{printf "%.2f", (s*8)/(d*1000*1000)}')

cat > "$OUT/result.txt" <<TXT
proto=${PROTO}
round=${ROUND}
url=${URL}
duration_seconds=${DUR}
bytes=${SIZE}
throughput_mbps=${MBPS}
sha256=${SHA}
rss_ready_kb=${RSS_READY_KB}
rss_warm_kb=${RSS_WARM_KB}
peak_rss_kb=${PEAK_RSS_KB}
rss_final_kb=${RSS_FINAL_KB}
path_routing_check=${PATH_ROUTING_CHECK}
TXT

rm -f "$OUT/blob.bin"
cat "$OUT/result.txt"
