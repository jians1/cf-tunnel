package runtime

import (
	"bytes"
	"testing"
)

func TestNewRuntimeConnectionOptionsIncludesConnectorClientID(t *testing.T) {
	t.Parallel()

	snapshot, err := newRuntimeConnectionOptions(nil)
	if err != nil {
		t.Fatalf("new runtime connection options: %v", err)
	}
	options := snapshot.ConnectionOptions()

	if len(options.Client.ClientID) != 16 {
		t.Fatalf("expected 16-byte client id, got %d bytes", len(options.Client.ClientID))
	}
	if options.Client.Version == "" {
		t.Fatal("expected client version")
	}
	if options.Client.Version != "2026.7.3" {
		t.Fatalf("unexpected client version: %s", options.Client.Version)
	}
	if options.Client.Version != runtimeClientVersion {
		t.Fatalf("client version %q diverged from runtimeClientVersion %q", options.Client.Version, runtimeClientVersion)
	}
	if options.Client.Arch == "" {
		t.Fatal("expected client arch")
	}
}

func TestNewRuntimeConnectionOptionsUsesRuntimeSnapshotType(t *testing.T) {
	t.Parallel()

	snapshot, err := newRuntimeConnectionOptions(nil)
	if err != nil {
		t.Fatalf("new runtime connection options: %v", err)
	}
	if _, ok := any(snapshot).(*runtimeConnectionOptionsSnapshot); !ok {
		t.Fatalf("expected runtime snapshot type, got %T", snapshot)
	}
}

func TestNewRuntimeConnectionOptionsUsesProvidedConnectorID(t *testing.T) {
	t.Parallel()

	connectorID := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	snapshot, err := newRuntimeConnectionOptions(connectorID)
	if err != nil {
		t.Fatalf("new runtime connection options: %v", err)
	}
	options := snapshot.ConnectionOptions()

	if !bytes.Equal(options.Client.ClientID, connectorID) {
		t.Fatalf("unexpected client id: got %v want %v", options.Client.ClientID, connectorID)
	}
}

func TestNewRuntimeConnectionOptionsCopiesProvidedConnectorID(t *testing.T) {
	t.Parallel()

	connectorID := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	snapshot, err := newRuntimeConnectionOptions(connectorID)
	if err != nil {
		t.Fatalf("new runtime connection options: %v", err)
	}
	connectorID[0] = 99

	if snapshot.ConnectionOptions().Client.ClientID[0] == 99 {
		t.Fatal("expected connector id to be copied")
	}
}
