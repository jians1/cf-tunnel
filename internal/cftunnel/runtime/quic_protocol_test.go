package runtime

import (
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
