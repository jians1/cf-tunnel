package runtime

import tunnelpogs "github.com/cloudflare/cloudflared/tunnelrpc/pogs"

type runtimeConnectRequest = tunnelpogs.ConnectRequest
type runtimeConnectResponse = tunnelpogs.ConnectResponse
type runtimeMetadata = tunnelpogs.Metadata
type runtimeConnectionType = tunnelpogs.ConnectionType

const (
	runtimeConnectionTypeHTTP      runtimeConnectionType = tunnelpogs.ConnectionTypeHTTP
	runtimeConnectionTypeWebsocket runtimeConnectionType = tunnelpogs.ConnectionTypeWebsocket
	runtimeConnectionTypeTCP       runtimeConnectionType = tunnelpogs.ConnectionTypeTCP
)

var runtimeErrorFlowConnectRateLimitedMetadata = runtimeMetadata{
	Key: "FlowConnectRateLimited",
	Val: "true",
}
