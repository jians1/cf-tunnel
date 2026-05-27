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
		`OUT="$BASE/${PROTO}-round${ROUND}"`,
	}

	for _, needle := range required {
		if !strings.Contains(text, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
}
