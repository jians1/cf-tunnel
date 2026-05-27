#!/usr/bin/env bash
set -euo pipefail

PROTO="${1:?proto required: http2 or quic}"
ROUND="${2:?round required}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BASE="${TMPDIR:-/tmp}/cfqt-e2e"
ORIGIN_DIR="$BASE/origin"
HTTP_PORT=$((18080 + ROUND))
WS_PORT=$((10000 + ROUND))
SOCKS_PORT=$((1080 + ROUND))
CFQT_BIN="$BASE/cfqt"
SING_BIN="${SING_BOX_BIN:-/root/.local/bin/sing-box}"
UUID="${SING_BOX_UUID:-ff78bef5-223f-4845-8676-a2780c305ea4}"
OUT="$BASE/${PROTO}-round${ROUND}"

mkdir -p "$OUT" "$ORIGIN_DIR"

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
  (cd "$ROOT_DIR" && go build -buildvcs=false -o "$CFQT_BIN" ./cmd/app)
fi

truncate -s 1G "$ORIGIN_DIR/blob.bin"

cat > "$BASE/sing-box-server.json" <<JSON
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

"$SING_BIN" run -c "$BASE/sing-box-server.json" >"$OUT/sing-server.log" 2>&1 &
PIDS+=($!)

"$CFQT_BIN" \
  --enable-cf-tunnel \
  --cf-edge-protocol="$PROTO" \
  --cf-tunnel-target=127.0.0.1:${WS_PORT} \
  --cf-origin-protocol=ws \
  --health-listen= \
  --log-level=info \
  >"$OUT/cfqt.log" 2>&1 &
CFQT_PID=$!
PIDS+=($CFQT_PID)

URL=""
for _ in $(seq 1 60); do
  if grep -Eo 'https://[-a-z0-9]+\.trycloudflare\.com' "$OUT/cfqt.log" >/dev/null 2>&1; then
    URL="$(grep -Eo 'https://[-a-z0-9]+\.trycloudflare\.com' "$OUT/cfqt.log" | tail -n1)"
    break
  fi
  sleep 1
done
if [[ -z "$URL" ]]; then
  echo "failed to obtain quick tunnel url" >&2
  exit 1
fi

HOST="${URL#https://}"
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
      "server": "${HOST}",
      "server_port": 443,
      "uuid": "${UUID}",
      "tls": {"enabled": true, "server_name": "${HOST}"},
      "transport": {"type": "ws", "path": "/ws"}
    }
  ]
}
JSON

"$SING_BIN" run -c "$OUT/sing-box-client.json" >"$OUT/sing-client.log" 2>&1 &
PIDS+=($!)

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

(
  while kill -0 "$CFQT_PID" 2>/dev/null; do
    ps -o rss= -p "$CFQT_PID" | awk '{print strftime("%s"), $1}' >> "$OUT/rss.log"
    sleep 1
  done
) &
PIDS+=($!)

START=$(date +%s)
curl -fsS --socks5-hostname 127.0.0.1:${SOCKS_PORT} "http://127.0.0.1:${HTTP_PORT}/blob.bin" -o "$OUT/blob.bin"
END=$(date +%s)

SHA=$(sha256sum "$OUT/blob.bin" | awk '{print $1}')
PEAK_RSS_KB=$(awk 'BEGIN{m=0} {if ($2>m) m=$2} END{print m+0}' "$OUT/rss.log")
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
peak_rss_kb=${PEAK_RSS_KB}
TXT

rm -f "$OUT/blob.bin"
cat "$OUT/result.txt"
