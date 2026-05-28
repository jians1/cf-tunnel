package runtime

import "testing"

func TestParseEdgeProtocolAcceptsSupportedValues(t *testing.T) {
	t.Parallel()

	tests := []string{"http2", "quic"}
	for _, tt := range tests {
		tt := tt
		t.Run(tt, func(t *testing.T) {
			t.Parallel()

			got, err := ParseEdgeProtocol(tt)
			if err != nil {
				t.Fatalf("parse edge protocol: %v", err)
			}
			if got.String() != tt {
				t.Fatalf("unexpected protocol: %s", got)
			}
		})
	}
}

func TestParseEdgeProtocolRejectsUnsupportedValue(t *testing.T) {
	t.Parallel()

	if _, err := ParseEdgeProtocol("auto"); err == nil {
		t.Fatal("expected unsupported protocol error")
	}
}

