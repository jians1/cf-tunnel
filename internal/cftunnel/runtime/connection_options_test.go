package runtime

import "testing"

func TestNewRuntimeConnectionOptionsIncludesConnectorClientID(t *testing.T) {
	t.Parallel()

	snapshot, err := newRuntimeConnectionOptions()
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
	if options.Client.Arch == "" {
		t.Fatal("expected client arch")
	}
}
