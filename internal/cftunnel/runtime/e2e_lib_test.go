package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ELibExtractURLSupportsCurrentAndLegacyLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		logLine  string
		selector string
		want     string
	}{
		{
			name:    "current quick tunnel ready log",
			logLine: `{"time":"2026-06-05T10:18:42.887243308+08:00","level":"INFO","msg":"quick tunnel ready","component":"cftunnel","url":"https://demo-current.trycloudflare.com","protocol":"quic","origin":"http://127.0.0.1:18081"}`,
			want:    "https://demo-current.trycloudflare.com",
		},
		{
			name:     "current multi tunnel log with selector",
			logLine:  `{"time":"2026-06-05T10:21:03.986350013+08:00","level":"INFO","msg":"quick tunnel ready","component":"cftunnel","tunnel_name":"beta","url":"https://demo-beta.trycloudflare.com","protocol":"http2","origin":"http://127.0.0.1:18182"}`,
			selector: `"tunnel_name":"beta"`,
			want:     "https://demo-beta.trycloudflare.com",
		},
		{
			name:    "legacy startup summary log",
			logLine: `{"msg":"cftunnel startup summary","quick_tunnel_url":"https://demo-legacy.trycloudflare.com"}`,
			want:    "https://demo-legacy.trycloudflare.com",
		},
	}

	libPath := filepath.Join("..", "..", "..", "scripts", "e2e", "lib.sh")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()
			logPath := filepath.Join(tmp, "app.log")
			if err := os.WriteFile(logPath, []byte(tt.logLine+"\n"), 0o644); err != nil {
				t.Fatalf("write log file: %v", err)
			}

			cmd := exec.Command("bash", "-lc", buildExtractURLCommand(libPath, logPath, tt.selector))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run extract url command: %v\noutput=%s", err, out)
			}

			got := strings.TrimSpace(string(out))
			if got != tt.want {
				t.Fatalf("unexpected url: got %q want %q", got, tt.want)
			}
		})
	}
}

func buildExtractURLCommand(libPath, logPath, selector string) string {
	if selector == "" {
		return "source " + shellQuote(libPath) + " && cfqt_extract_url " + shellQuote(logPath)
	}
	return "source " + shellQuote(libPath) + " && cfqt_extract_url " + shellQuote(logPath) + " " + shellQuote(selector)
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}
