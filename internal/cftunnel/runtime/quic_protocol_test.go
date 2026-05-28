package runtime

import (
	"errors"
	"net"
	"testing"

	"github.com/cloudflare/cloudflared/tunnelrpc/proto"
	capnp "zombiezen.com/go/capnproto2"
)

func TestRuntimeConnectRequestRoundTrip(t *testing.T) {
	t.Parallel()

	original := &runtimeConnectRequest{
		Dest: "https://example.com/path",
		Type: runtimeConnectionTypeWebsocket,
		Metadata: []runtimeMetadata{
			{Key: quicHTTPHeaderKey + ":Host", Val: "example.com"},
			{Key: quicMetadataFlowIDKey, Val: "flow-1"},
		},
	}

	msg, err := original.ToPogs()
	if err != nil {
		t.Fatalf("encode connect request: %v", err)
	}

	var decoded runtimeConnectRequest
	if err := decoded.FromPogs(msg); err != nil {
		t.Fatalf("decode connect request: %v", err)
	}

	if decoded.Dest != original.Dest {
		t.Fatalf("unexpected dest: got %q want %q", decoded.Dest, original.Dest)
	}
	if decoded.Type != original.Type {
		t.Fatalf("unexpected type: got %v want %v", decoded.Type, original.Type)
	}
	metadata := decoded.MetadataMap()
	if metadata[quicHTTPHeaderKey+":Host"] != "example.com" {
		t.Fatalf("unexpected host metadata: %v", metadata)
	}
	if metadata[quicMetadataFlowIDKey] != "flow-1" {
		t.Fatalf("unexpected flow metadata: %v", metadata)
	}
}

func TestRuntimeConnectResponseRoundTrip(t *testing.T) {
	t.Parallel()

	original := &runtimeConnectResponse{
		Error: "origin unavailable",
		Metadata: []runtimeMetadata{
			{Key: quicHTTPStatusKey, Val: "503"},
			runtimeErrorFlowConnectRateLimitedMetadata,
		},
	}

	msg, err := original.ToPogs()
	if err != nil {
		t.Fatalf("encode connect response: %v", err)
	}

	var decoded runtimeConnectResponse
	if err := decoded.FromPogs(msg); err != nil {
		t.Fatalf("decode connect response: %v", err)
	}

	if decoded.Error != original.Error {
		t.Fatalf("unexpected error: got %q want %q", decoded.Error, original.Error)
	}
	if len(decoded.Metadata) != len(original.Metadata) {
		t.Fatalf("unexpected metadata length: got %d want %d", len(decoded.Metadata), len(original.Metadata))
	}
	if decoded.Metadata[0] != original.Metadata[0] {
		t.Fatalf("unexpected status metadata: got %+v want %+v", decoded.Metadata[0], original.Metadata[0])
	}
	if decoded.Metadata[1] != original.Metadata[1] {
		t.Fatalf("unexpected rate limit metadata: got %+v want %+v", decoded.Metadata[1], original.Metadata[1])
	}
}

func TestRuntimeRegistrationOptionsMarshal(t *testing.T) {
	t.Parallel()

	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("new capnp message: %v", err)
	}
	_ = msg
	options, err := proto.NewConnectionOptions(seg)
	if err != nil {
		t.Fatalf("new connection options: %v", err)
	}

	runtimeOptions := &runtimeConnectionOptions{
		Client: runtimeClientInfo{
			ClientID: []byte("1234567890123456"),
			Features: []string{"serialized_headers"},
			Version:  "test",
			Arch:     "linux_amd64",
		},
		OriginLocalIP:       net.IPv4(127, 0, 0, 1),
		ReplaceExisting:     true,
		CompressionQuality:  3,
		NumPreviousAttempts: 2,
	}

	if err := runtimeOptions.marshalCapnproto(options); err != nil {
		t.Fatalf("marshal runtime connection options: %v", err)
	}
}

func TestRuntimeRegisterUDPSessionResponseMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  runtimeRegisterUDPSessionResponse
		wantErr   string
		wantSpans []byte
	}{
		{
			name: "success preserves spans",
			response: runtimeRegisterUDPSessionResponse{
				Spans: []byte("trace-spans"),
			},
			wantSpans: []byte("trace-spans"),
		},
		{
			name: "error preserves message and omits spans",
			response: runtimeRegisterUDPSessionResponse{
				Err:   errors.New("session rejected"),
				Spans: []byte("ignored"),
			},
			wantErr: "session rejected",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
			if err != nil {
				t.Fatalf("new capnp message: %v", err)
			}
			result, err := proto.NewRegisterUdpSessionResponse(seg)
			if err != nil {
				t.Fatalf("new register UDP session response: %v", err)
			}

			if err := test.response.marshalCapnproto(result); err != nil {
				t.Fatalf("marshal register UDP session response: %v", err)
			}

			gotErr, err := result.Err()
			if err != nil {
				t.Fatalf("read response error: %v", err)
			}
			if gotErr != test.wantErr {
				t.Fatalf("unexpected error: got %q want %q", gotErr, test.wantErr)
			}

			gotSpans, err := result.Spans()
			if err != nil {
				t.Fatalf("read response spans: %v", err)
			}
			if string(gotSpans) != string(test.wantSpans) {
				t.Fatalf("unexpected spans: got %q want %q", gotSpans, test.wantSpans)
			}
		})
	}
}

func TestRuntimeUpdateConfigurationResponseMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		response    runtimeUpdateConfigurationResponse
		wantVersion int32
		wantErr     string
	}{
		{
			name: "success preserves version",
			response: runtimeUpdateConfigurationResponse{
				LastAppliedVersion: 42,
			},
			wantVersion: 42,
		},
		{
			name: "error preserves message",
			response: runtimeUpdateConfigurationResponse{
				LastAppliedVersion: 7,
				Err:                errors.New("bad config"),
			},
			wantVersion: 7,
			wantErr:     "bad config",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
			if err != nil {
				t.Fatalf("new capnp message: %v", err)
			}
			result, err := proto.NewUpdateConfigurationResponse(seg)
			if err != nil {
				t.Fatalf("new update configuration response: %v", err)
			}

			if err := test.response.marshalCapnproto(result); err != nil {
				t.Fatalf("marshal update configuration response: %v", err)
			}

			if gotVersion := result.LatestAppliedVersion(); gotVersion != test.wantVersion {
				t.Fatalf("unexpected version: got %d want %d", gotVersion, test.wantVersion)
			}

			gotErr := ""
			if result.HasErr() {
				var err error
				gotErr, err = result.Err()
				if err != nil {
					t.Fatalf("read response error: %v", err)
				}
			}
			if gotErr != test.wantErr {
				t.Fatalf("unexpected error: got %q want %q", gotErr, test.wantErr)
			}
		})
	}
}
