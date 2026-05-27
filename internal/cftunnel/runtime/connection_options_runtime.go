package runtime

import (
	"crypto/rand"
	"fmt"
	"net"
	goruntime "runtime"

	"github.com/cloudflare/cloudflared/tunnelrpc/pogs"
)

const runtimeClientVersion = "cf-quicktunnel-ipv6pool/0.1.0-prototype"

type runtimeConnectionOptionsSnapshot struct {
	client              pogs.ClientInfo
	originLocalIP       net.IP
	numPreviousAttempts uint8
}

func newRuntimeConnectionOptions() (*runtimeConnectionOptionsSnapshot, error) {
	clientID := make([]byte, 16)
	if _, err := rand.Read(clientID); err != nil {
		return nil, fmt.Errorf("generate connector id: %w", err)
	}

	return &runtimeConnectionOptionsSnapshot{
		client: pogs.ClientInfo{
			ClientID: clientID,
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

func (c *runtimeConnectionOptionsSnapshot) ConnectionOptions() *pogs.ConnectionOptions {
	return &pogs.ConnectionOptions{
		Client:              c.client,
		OriginLocalIP:       c.originLocalIP,
		ReplaceExisting:     false,
		CompressionQuality:  0,
		NumPreviousAttempts: c.numPreviousAttempts,
	}
}
