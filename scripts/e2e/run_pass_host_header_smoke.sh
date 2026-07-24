#!/usr/bin/env bash
# Real Quick Tunnel smoke for pass_host_header + X-Forwarded-Proto.
#
# Checks:
#   1) default: origin Host is local target host (not public hostname)
#   2) pass_host_header=true: origin Host equals public hostname
#   3) both paths set X-Forwarded-Proto: https and X-Forwarded-Host
#
# Exit codes:
#   0 pass
#   1 hard failure
#   2 assertion failure
#   3 rate limited by trycloudflare
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

ROOT_DIR="$(cfqt_root_dir)"
BASE="${TMPDIR:-/tmp}/cfqt-pass-host-smoke"
OUT="$BASE/out"
BIN="$BASE/cfqt"
ORIGIN_PORT=18191
ORIGIN_ADDR="127.0.0.1:${ORIGIN_PORT}"

rm -rf "$BASE"
mkdir -p "$OUT"

cleanup() {
  set +e
  for p in "${PIDS[@]:-}"; do kill "$p" 2>/dev/null || true; done
}
trap cleanup EXIT
PIDS=()

cfqt_build_binary "$ROOT_DIR" "$BIN"

# Origin that returns the headers we care about as JSON text.
cat > "$OUT/origin_server.py" <<'PY'
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        payload = {
            "host": self.headers.get("Host", ""),
            "x_forwarded_host": self.headers.get("X-Forwarded-Host", ""),
            "x_forwarded_proto": self.headers.get("X-Forwarded-Proto", ""),
            "path": self.path,
        }
        body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        return

ThreadingHTTPServer(("127.0.0.1", int(__import__("os").environ["ORIGIN_PORT"])), Handler).serve_forever()
PY

ORIGIN_PORT="$ORIGIN_PORT" python3 "$OUT/origin_server.py" >"$OUT/origin.log" 2>&1 &
PIDS+=($!)
sleep 0.3

run_case() {
  local name="$1"
  local pass_host="$2"
  local app_log="$OUT/${name}-app.log"
  local cfg="$OUT/${name}.yaml"

  cat > "$cfg" <<YAML
log_format: json
health_listen: ""
cf_tunnel:
  edge_protocol: quic
  target: http://${ORIGIN_ADDR}
  pass_host_header: ${pass_host}
YAML

  "$BIN" --config="$cfg" >"$app_log" 2>&1 &
  local app_pid=$!
  PIDS+=("$app_pid")

  local url
  url="$(cfqt_wait_url "$app_log" "" 90 || true)"
  if [[ "$url" == "rate_limited" ]]; then
    echo "${name}_status=rate_limited"
    tail -n 80 "$app_log" || true
    kill "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
    return 3
  fi
  if [[ -z "$url" ]]; then
    echo "${name}_status=failed_no_url"
    tail -n 80 "$app_log" || true
    kill "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
    return 1
  fi

  local host="${url#https://}"
  host="${host%%/*}"
  local ip=""
  if ! ip="$(cfqt_wait_edge_ip "$host" 120)"; then
    echo "${name}_status=dns_not_published"
    echo "${name}_url=${url}"
    kill "$app_pid" 2>/dev/null || true
    wait "$app_pid" 2>/dev/null || true
    return 1
  fi

  local body=""
  local attempt
  for attempt in $(seq 1 40); do
    body="$(cfqt_curl_https_resolved "$url" "$ip" 2>/dev/null || true)"
    if [[ -n "$body" && "$body" == *"x_forwarded_proto"* ]]; then
      break
    fi
    sleep 2
  done

  kill "$app_pid" 2>/dev/null || true
  wait "$app_pid" 2>/dev/null || true

  if [[ -z "$body" ]]; then
    echo "${name}_status=http_empty"
    echo "${name}_url=${url}"
    echo "${name}_dns_ip=${ip}"
    return 1
  fi

  local seen_host seen_xfh seen_xfp
  seen_host="$(printf '%s' "$body" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("host",""))')"
  seen_xfh="$(printf '%s' "$body" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("x_forwarded_host",""))')"
  seen_xfp="$(printf '%s' "$body" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("x_forwarded_proto",""))')"

  echo "${name}_url=${url}"
  echo "${name}_dns_ip=${ip}"
  echo "${name}_seen_host=${seen_host}"
  echo "${name}_seen_x_forwarded_host=${seen_xfh}"
  echo "${name}_seen_x_forwarded_proto=${seen_xfp}"
  echo "${name}_body=${body}"

  local ok=1
  if [[ "$seen_xfp" != "https" ]]; then
    echo "${name}_assert=x_forwarded_proto_want_https_got_${seen_xfp}"
    ok=0
  fi
  if [[ "$seen_xfh" != "$host" ]]; then
    echo "${name}_assert=x_forwarded_host_want_${host}_got_${seen_xfh}"
    ok=0
  fi
  if [[ "$pass_host" == "true" ]]; then
    if [[ "$seen_host" != "$host" ]]; then
      echo "${name}_assert=host_want_public_${host}_got_${seen_host}"
      ok=0
    fi
  else
    # default: Host rewritten to local target host:port
    if [[ "$seen_host" != "$ORIGIN_ADDR" ]]; then
      echo "${name}_assert=host_want_local_${ORIGIN_ADDR}_got_${seen_host}"
      ok=0
    fi
  fi

  echo "${name}_ok=${ok}"
  if [[ "$ok" -ne 1 ]]; then
    return 2
  fi
  return 0
}

DEFAULT_RC=0
PASS_RC=0

run_case "default" "false" || DEFAULT_RC=$?
if [[ "$DEFAULT_RC" -eq 3 ]]; then
  echo "smoke_result=rate_limited"
  exit 3
fi

run_case "pass_host" "true" || PASS_RC=$?
if [[ "$PASS_RC" -eq 3 ]]; then
  echo "smoke_result=rate_limited"
  exit 3
fi

echo "default_rc=${DEFAULT_RC}"
echo "pass_host_rc=${PASS_RC}"

if [[ "$DEFAULT_RC" -ne 0 || "$PASS_RC" -ne 0 ]]; then
  echo "smoke_result=failed"
  exit 2
fi
echo "smoke_result=pass"
