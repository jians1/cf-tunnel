package config

import (
	"testing"
)

type stubCLIContext struct {
	values map[string]string
	set    map[string]bool
	args   []string
}

func (c stubCLIContext) IsSet(name string) bool {
	return c.set[name]
}

func (c stubCLIContext) String(name string) string {
	return c.values[name]
}

func (c stubCLIContext) NArg() int {
	return len(c.args)
}

func (c stubCLIContext) Arg(i int) string {
	if i < 0 || i >= len(c.args) {
		return ""
	}
	return c.args[i]
}

func TestValidateURLWithMinimalCLIContext(t *testing.T) {
	t.Parallel()

	ctx := stubCLIContext{
		values: map[string]string{"url": "http://127.0.0.1:8080"},
		set:    map[string]bool{"url": true},
	}

	u, err := ValidateUrl(ctx, false)
	if err != nil {
		t.Fatalf("validate url: %v", err)
	}
	if got := u.String(); got != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected url: %s", got)
	}
}

func TestValidateUnixSocketWithMinimalCLIContext(t *testing.T) {
	t.Parallel()

	ctx := stubCLIContext{
		values: map[string]string{"unix-socket": "/tmp/demo.sock"},
		set:    map[string]bool{"unix-socket": true},
	}

	socket, err := ValidateUnixSocket(ctx)
	if err != nil {
		t.Fatalf("validate unix socket: %v", err)
	}
	if socket != "/tmp/demo.sock" {
		t.Fatalf("unexpected unix socket: %s", socket)
	}
}
