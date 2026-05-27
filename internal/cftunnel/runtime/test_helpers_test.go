package runtime

import (
	"net/url"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}
