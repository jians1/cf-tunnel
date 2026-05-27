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
CF_DNS_RESOLVER="${CF_DNS_RESOLVER:-1.1.1.1}"
CFQT_BIN="$BASE/cfqt"
SING_BIN="${SING_BOX_BIN:-/root/.local/bin/sing-box}"
UUID="${SING_BOX_UUID:-ff78bef5-223f-4845-8676-a2780c305ea4}"
OUT="$BASE/${PROTO}-round${ROUND}"
PHASE_FILE="$OUT/phase"

rm -rf "$BASE"
mkdir -p "$OUT" "$ORIGIN_DIR"
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
  (cd "$ROOT_DIR" && CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o "$CFQT_BIN" ./cmd/app)
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
EDGE_IP=""
for _ in $(seq 1 30); do
  EDGE_IP="$(curl -fsS -H 'accept: application/dns-json' "https://${CF_DNS_RESOLVER}/dns-query?name=${HOST}&type=A" \
    | python3 -c 'import json,sys; data=json.load(sys.stdin); answers=data.get("Answer") or []; print(next((item["data"] for item in answers if item.get("type")==1), ""))' \
    2>/dev/null || true)"
  if [[ -n "$EDGE_IP" ]]; then
    break
  fi
  EDGE_IP="$(curl -fsS -H 'accept: application/dns-json' "https://${CF_DNS_RESOLVER}/dns-query?name=${HOST}&type=AAAA" \
    | python3 -c 'import json,sys; data=json.load(sys.stdin); answers=data.get("Answer") or []; print(next((item["data"] for item in answers if item.get("type")==28), ""))' \
    2>/dev/null || true)"
  if [[ -n "$EDGE_IP" ]]; then
    break
  fi
  sleep 1
done
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
TXT

rm -f "$OUT/blob.bin"
cat "$OUT/result.txt"
