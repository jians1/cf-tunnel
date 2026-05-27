package runtime

import tunnelpogs "github.com/cloudflare/cloudflared/tunnelrpc/pogs"

type runtimeConnectRequest = tunnelpogs.ConnectRequest
type runtimeConnectResponse = tunnelpogs.ConnectResponse
type runtimeMetadata = tunnelpogs.Metadata
type runtimeConnectionType = tunnelpogs.ConnectionType
type runtimeRegisterUDPSessionResponse = tunnelpogs.RegisterUdpSessionResponse
type runtimeUpdateConfigurationResponse = tunnelpogs.UpdateConfigurationResponse
type runtimeClientInfo = tunnelpogs.ClientInfo
type runtimeConnectionOptions = tunnelpogs.ConnectionOptions
type runtimeTunnelAuth = tunnelpogs.TunnelAuth
type runtimeConnectionDetails = tunnelpogs.ConnectionDetails

const (
	runtimeConnectionTypeHTTP      runtimeConnectionType = tunnelpogs.ConnectionTypeHTTP
	runtimeConnectionTypeWebsocket runtimeConnectionType = tunnelpogs.ConnectionTypeWebsocket
	runtimeConnectionTypeTCP       runtimeConnectionType = tunnelpogs.ConnectionTypeTCP
)

var runtimeErrorFlowConnectRateLimitedMetadata = runtimeMetadata{
	Key: "FlowConnectRateLimited",
	Val: "true",
}
