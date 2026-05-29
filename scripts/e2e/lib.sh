#!/usr/bin/env bash

cfqt_root_dir() {
  cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd
}

cfqt_build_binary() {
  local root_dir="$1"
  local output="$2"
  (cd "$root_dir" && CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o "$output" ./cmd/app)
}

cfqt_extract_url() {
  local log="$1"
  local selector="${2:-}"
  if [[ -n "$selector" ]]; then
    grep "$selector" "$log" | grep 'cftunnel startup summary' | tail -n1 | sed -n 's/.*"quick_tunnel_url":"\([^"]*\)".*/\1/p'
  else
    grep 'cftunnel startup summary' "$log" | tail -n1 | sed -n 's/.*"quick_tunnel_url":"\([^"]*\)".*/\1/p'
  fi
}

cfqt_detect_rate_limit() {
  local log="$1"
  grep -Eq 'status=429 Too Many Requests|error code: 1015|rate limited' "$log"
}

cfqt_wait_url() {
  local log="$1"
  local selector="${2:-}"
  local attempts="${3:-80}"
  local url=""
  for _ in $(seq 1 "$attempts"); do
    url="$(cfqt_extract_url "$log" "$selector" || true)"
    if [[ -n "$url" ]]; then
      echo "$url"
      return 0
    fi
    if cfqt_detect_rate_limit "$log"; then
      echo "rate_limited"
      return 2
    fi
    sleep 1
  done
  return 1
}

cfqt_lookup_a() {
  local host="$1"
  curl -fsS --max-time 10 -H 'accept: application/dns-json' \
    "https://cloudflare-dns.com/dns-query?name=${host}&type=A" \
    | python3 -c 'import json,sys; data=json.load(sys.stdin); answers=data.get("Answer") or []; print(next((item["data"] for item in answers if item.get("type")==1), ""))'
}

cfqt_lookup_aaaa() {
  local host="$1"
  curl -fsS --max-time 10 -H 'accept: application/dns-json' \
    "https://cloudflare-dns.com/dns-query?name=${host}&type=AAAA" \
    | python3 -c 'import json,sys; data=json.load(sys.stdin); answers=data.get("Answer") or []; print(next((item["data"] for item in answers if item.get("type")==28), ""))'
}

cfqt_wait_edge_ip() {
  local host="$1"
  local attempts="${2:-120}"
  local ip=""
  for _ in $(seq 1 "$attempts"); do
    ip="$(cfqt_lookup_a "$host" 2>/dev/null || true)"
    if [[ -n "$ip" ]]; then
      echo "$ip"
      return 0
    fi
    ip="$(cfqt_lookup_aaaa "$host" 2>/dev/null || true)"
    if [[ -n "$ip" ]]; then
      echo "$ip"
      return 0
    fi
    sleep 2
  done
  return 1
}

cfqt_curl_https_resolved() {
  local url="$1"
  local ip="$2"
  local host="${url#https://}"
  curl -fsSL --max-time 10 --resolve "${host}:443:${ip}" "$url"
}

cfqt_check_http_body() {
  local name="$1"
  local url="$2"
  local want="$3"
  local host="${url#https://}"
  local ip=""
  local body=""

  if ! ip="$(cfqt_wait_edge_ip "$host")"; then
    echo "${name}_status=dns_not_published"
    echo "${name}_ok=0"
    return 1
  fi

  for _ in $(seq 1 30); do
    body="$(cfqt_curl_https_resolved "$url" "$ip" 2>/dev/null || true)"
    if [[ "$body" == *"$want"* ]]; then
      echo "${name}_status=pass"
      echo "${name}_dns_ip=${ip}"
      echo "${name}_ok=1"
      return 0
    fi
    sleep 2
  done

  echo "${name}_status=http_mismatch"
  echo "${name}_dns_ip=${ip}"
  echo "${name}_body=${body}"
  echo "${name}_ok=0"
  return 1
}
