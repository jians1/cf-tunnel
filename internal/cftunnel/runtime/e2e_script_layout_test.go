package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EScriptTemplateUsesRoundScopedPortsAndCleanup(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "scripts", "e2e", "run_trycloudflare_ab.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	text := string(data)

	required := []string{
		"cleanup()",
		"trap cleanup EXIT",
		`ROUND="${2:?round required}"`,
		`case "$PROTO" in`,
		`http2) PROTO_PORT_OFFSET=0 ;;`,
		`quic) PROTO_PORT_OFFSET=100 ;;`,
		`HTTP_PORT=$((18080 + PROTO_PORT_OFFSET + ROUND))`,
		`WS_PORT=$((10000 + PROTO_PORT_OFFSET + ROUND))`,
		`SOCKS_PORT=$((1080 + PROTO_PORT_OFFSET + ROUND))`,
		`rm -rf "$OUT"`,
		`OUT="$BASE/${PROTO}-round${ROUND}"`,
		`source "$SCRIPT_DIR/lib.sh"`,
		`URL="$(cfqt_wait_url "$OUT/cfqt.log" "" 60 || true)"`,
		`EDGE_IP="$(cfqt_wait_edge_ip "$HOST" 30 || true)"`,
		`"server": "${EDGE_IP}"`,
		`"server_name": "${HOST}"`,
		`"headers": {"Host": "${HOST}"}`,
		`PHASE_FILE="$OUT/phase"`,
		`echo "startup" > "$PHASE_FILE"`,
		`echo "ready" > "$PHASE_FILE"`,
		`echo "warm" > "$PHASE_FILE"`,
		`echo "download" > "$PHASE_FILE"`,
		`echo "final" > "$PHASE_FILE"`,
		`rss_ready_kb=`,
		`rss_warm_kb=`,
		`peak_rss_kb=`,
		`rss_final_kb=`,
	}

	for _, needle := range required {
		if !strings.Contains(text, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
}
