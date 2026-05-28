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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BASE="${TMPDIR:-/tmp}/cfqt-e2e"
CFQT_BIN="$BASE/cfqt"
OUT="$BASE/multi-${PROTO}-round${ROUND}"
PHASE_FILE="$OUT/phase"
CF_DNS_RESOLVER="${CF_DNS_RESOLVER:-1.1.1.1}"

ORIGIN1_DIR="$OUT/origin-1"
ORIGIN2_DIR="$OUT/origin-2"
HTTP1_PORT=$((38080 + PROTO_PORT_OFFSET + ROUND))
HTTP2_PORT=$((38180 + PROTO_PORT_OFFSET + ROUND))

rm -rf "$OUT"
mkdir -p "$OUT" "$ORIGIN1_DIR" "$ORIGIN2_DIR"
echo "startup" > "$PHASE_FILE"

cleanup() {
  set +e
  for p in "${PIDS[@]:-}"; do kill "$p" 2>/dev/null || true; done
}
trap cleanup EXIT
PIDS=()

(cd "$ROOT_DIR" && CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o "$CFQT_BIN" ./cmd/app)

truncate -s 256M "$ORIGIN1_DIR/blob.bin"
truncate -s 256M "$ORIGIN2_DIR/blob.bin"
echo "t1-ok" > "$ORIGIN1_DIR/id.txt"
echo "t2-ok" > "$ORIGIN2_DIR/id.txt"

python3 -m http.server "$HTTP1_PORT" --bind 127.0.0.1 --directory "$ORIGIN1_DIR" >"$OUT/http-origin-1.log" 2>&1 &
PIDS+=($!)
python3 -m http.server "$HTTP2_PORT" --bind 127.0.0.1 --directory "$ORIGIN2_DIR" >"$OUT/http-origin-2.log" 2>&1 &
PIDS+=($!)

"$CFQT_BIN" \
  --cf-tunnel=name=t1,target=127.0.0.1:${HTTP1_PORT},origin=http,edge=${PROTO} \
  --cf-tunnel=name=t2,target=127.0.0.1:${HTTP2_PORT},origin=http,edge=${PROTO} \
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

URL1=""
URL2=""
for _ in $(seq 1 90); do
  mapfile -t URLS < <(grep -Eo 'https://[-a-z0-9]+\.trycloudflare\.com' "$OUT/cfqt.log" | awk '!seen[$0]++')
  if [[ "${#URLS[@]}" -ge 2 ]]; then
    URL1="${URLS[0]}"
    URL2="${URLS[1]}"
    break
  fi
  sleep 1
done
if [[ -z "$URL1" || -z "$URL2" ]]; then
  echo "failed to obtain two quick tunnel urls" >&2
  exit 1
fi

resolve_host_ip() {
  local host="$1"
  local ip=""
  ip="$(curl -fsS -H 'accept: application/dns-json' "https://${CF_DNS_RESOLVER}/dns-query?name=${host}&type=A" \
    | python3 -c 'import json,sys; data=json.load(sys.stdin); answers=data.get("Answer") or []; print(next((item["data"] for item in answers if item.get("type")==1), ""))' \
    2>/dev/null || true)"
  if [[ -n "$ip" ]]; then
    echo "$ip"
    return 0
  fi
  ip="$(curl -fsS -H 'accept: application/dns-json' "https://${CF_DNS_RESOLVER}/dns-query?name=${host}&type=AAAA" \
    | python3 -c 'import json,sys; data=json.load(sys.stdin); answers=data.get("Answer") or []; print(next((item["data"] for item in answers if item.get("type")==28), ""))' \
    2>/dev/null || true)"
  if [[ -n "$ip" ]]; then
    echo "$ip"
    return 0
  fi
  return 1
}

for host in "${URL1#https://}" "${URL2#https://}"; do
  RESOLVED=0
  for _ in $(seq 1 30); do
    if resolve_host_ip "$host" >/dev/null; then
      RESOLVED=1
      break
    fi
    sleep 1
  done
  if [[ "$RESOLVED" -ne 1 ]]; then
    echo "failed to resolve quick tunnel hostname: $host" >&2
    exit 1
  fi
done

echo "ready" > "$PHASE_FILE"
sleep 2

for _ in $(seq 1 30); do
  if [[ "$(curl -fsS --retry 2 --retry-delay 1 --retry-connrefused "${URL1}/id.txt" || true)" == "t1-ok" ]] && [[ "$(curl -fsS --retry 2 --retry-delay 1 --retry-connrefused "${URL2}/id.txt" || true)" == "t2-ok" ]]; then
    break
  fi
  sleep 2
done

ID1="$(curl -fsS --retry 2 --retry-delay 1 --retry-connrefused "${URL1}/id.txt")"
ID2="$(curl -fsS --retry 2 --retry-delay 1 --retry-connrefused "${URL2}/id.txt")"

T1_URL=""
T2_URL=""
if [[ "$ID1" == "t1-ok" ]]; then T1_URL="$URL1"; fi
if [[ "$ID1" == "t2-ok" ]]; then T2_URL="$URL1"; fi
if [[ "$ID2" == "t1-ok" ]]; then T1_URL="$URL2"; fi
if [[ "$ID2" == "t2-ok" ]]; then T2_URL="$URL2"; fi

if [[ -z "$T1_URL" || -z "$T2_URL" ]]; then
  echo "multi-tunnel check failed: unexpected id responses id1=${ID1} id2=${ID2}" >&2
  exit 1
fi
MULTI_TUNNEL_CHECK="pass"

echo "warm" > "$PHASE_FILE"
sleep 1

echo "download_t1" > "$PHASE_FILE"
START1=$(date +%s)
curl -fsS "${T1_URL}/blob.bin" -o "$OUT/blob1.bin"
END1=$(date +%s)

echo "download_t2" > "$PHASE_FILE"
START2=$(date +%s)
curl -fsS "${T2_URL}/blob.bin" -o "$OUT/blob2.bin"
END2=$(date +%s)

echo "final" > "$PHASE_FILE"
sleep 1

SHA1=$(sha256sum "$OUT/blob1.bin" | awk '{print $1}')
SHA2=$(sha256sum "$OUT/blob2.bin" | awk '{print $1}')
SIZE1=$(stat -c %s "$OUT/blob1.bin")
SIZE2=$(stat -c %s "$OUT/blob2.bin")
DUR1=$((END1-START1))
DUR2=$((END2-START2))
if [[ "$DUR1" -le 0 ]]; then DUR1=1; fi
if [[ "$DUR2" -le 0 ]]; then DUR2=1; fi
MBPS1=$(awk -v s="$SIZE1" -v d="$DUR1" 'BEGIN{printf "%.2f", (s*8)/(d*1000*1000)}')
MBPS2=$(awk -v s="$SIZE2" -v d="$DUR2" 'BEGIN{printf "%.2f", (s*8)/(d*1000*1000)}')

RSS_READY_KB=$(awk '$2=="ready" {v=$3} END{print v+0}' "$OUT/rss.log")
RSS_WARM_KB=$(awk '$2=="warm" {v=$3} END{print v+0}' "$OUT/rss.log")
PEAK_RSS_KB=$(awk 'BEGIN{m=0} ($2=="download_t1" || $2=="download_t2") {if ($3>m) m=$3} END{print m+0}' "$OUT/rss.log")
RSS_FINAL_KB=$(awk '$2=="final" {v=$3} END{print v+0}' "$OUT/rss.log")

cat > "$OUT/result.txt" <<TXT
proto=${PROTO}
round=${ROUND}
tunnel_count=2
url_1=${T1_URL}
url_2=${T2_URL}
duration_seconds_1=${DUR1}
duration_seconds_2=${DUR2}
bytes_1=${SIZE1}
bytes_2=${SIZE2}
throughput_mbps_1=${MBPS1}
throughput_mbps_2=${MBPS2}
sha256_1=${SHA1}
sha256_2=${SHA2}
rss_ready_kb=${RSS_READY_KB}
rss_warm_kb=${RSS_WARM_KB}
peak_rss_kb=${PEAK_RSS_KB}
rss_final_kb=${RSS_FINAL_KB}
multi_tunnel_check=${MULTI_TUNNEL_CHECK}
TXT

rm -f "$OUT/blob1.bin" "$OUT/blob2.bin"
cat "$OUT/result.txt"
