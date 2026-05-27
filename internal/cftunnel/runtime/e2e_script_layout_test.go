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
		`HTTP_PORT=$((18080 + ROUND))`,
		`WS_PORT=$((10000 + ROUND))`,
		`SOCKS_PORT=$((1080 + ROUND))`,
		`rm -rf "$BASE"`,
		`OUT="$BASE/${PROTO}-round${ROUND}"`,
		`CF_DNS_RESOLVER="${CF_DNS_RESOLVER:-1.1.1.1}"`,
		`EDGE_IP=""`,
		`https://${CF_DNS_RESOLVER}/dns-query?name=${HOST}&type=A`,
		`https://${CF_DNS_RESOLVER}/dns-query?name=${HOST}&type=AAAA`,
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
