package runtime

import (
	"crypto/rand"
	"fmt"
	"net"
	goruntime "runtime"
)

// Cloudflare compares this connector version against cloudflared releases.
const runtimeClientVersion = "2026.6.0"
const runtimeConnectorIDLength = 16

type runtimeConnectionOptionsSnapshot struct {
	client              runtimeClientInfo
	originLocalIP       net.IP
	numPreviousAttempts uint8
}

func newRuntimeConnectorID() ([]byte, error) {
	clientID := make([]byte, runtimeConnectorIDLength)
	if _, err := rand.Read(clientID); err != nil {
		return nil, fmt.Errorf("generate connector id: %w", err)
	}
	return clientID, nil
}

func newRuntimeConnectionOptions(connectorID []byte) (*runtimeConnectionOptionsSnapshot, error) {
	clientID := connectorID
	if len(clientID) == 0 {
		generated, err := newRuntimeConnectorID()
		if err != nil {
			return nil, err
		}
		clientID = generated
	}
	if len(clientID) != runtimeConnectorIDLength {
		return nil, fmt.Errorf("connector id must be %d bytes, got %d", runtimeConnectorIDLength, len(clientID))
	}

	return &runtimeConnectionOptionsSnapshot{
		client: runtimeClientInfo{
			ClientID: append([]byte(nil), clientID...),
			Version:  runtimeClientVersion,
			Arch:     goruntime.GOOS + "_" + goruntime.GOARCH,
			Features: []string{
				"allow_remote_config",
				"serialized_headers",
				"support_datagram_v2",
				"support_quic_eof",
				"management_logs",
			},
		},
	}, nil
}

func (c *runtimeConnectionOptionsSnapshot) ConnectionOptions() *runtimeConnectionOptions {
	return &runtimeConnectionOptions{
		Client:              c.client,
		OriginLocalIP:       c.originLocalIP,
		ReplaceExisting:     false,
		CompressionQuality:  0,
		NumPreviousAttempts: c.numPreviousAttempts,
	}
}
