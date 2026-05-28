package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	tunnelpogs "github.com/cloudflare/cloudflared/tunnelrpc/pogs"
	"github.com/cloudflare/cloudflared/tunnelrpc/proto"
	"github.com/google/uuid"
	capnp "zombiezen.com/go/capnproto2"
	capnppogs "zombiezen.com/go/capnproto2/pogs"
	"zombiezen.com/go/capnproto2/rpc"
)

type runtimeSessionManager interface {
	RegisterUdpSession(context.Context, uuid.UUID, net.IP, uint16, time.Duration, string) (*runtimeRegisterUDPSessionResponse, error)
	UnregisterUdpSession(context.Context, uuid.UUID, string) error
}

type runtimeConfigurationManager interface {
	UpdateConfiguration(context.Context, int32, []byte) *runtimeUpdateConfigurationResponse
}

type runtimeClientInfo struct {
	ClientID []byte `capnp:"clientId"`
	Features []string
	Version  string
	Arch     string
}

type runtimeConnectionOptions struct {
	Client              runtimeClientInfo
	OriginLocalIP       net.IP `capnp:"originLocalIp"`
	ReplaceExisting     bool
	CompressionQuality  uint8
	NumPreviousAttempts uint8
}

func (o *runtimeConnectionOptions) marshalCapnproto(s proto.ConnectionOptions) error {
	return capnppogs.Insert(proto.ConnectionOptions_TypeID, s.Struct, o)
}

type runtimeTunnelAuth struct {
	AccountTag   string
	TunnelSecret []byte
}

func (a runtimeTunnelAuth) marshalCapnproto(s proto.TunnelAuth) error {
	return capnppogs.Insert(proto.TunnelAuth_TypeID, s.Struct, &a)
}

type runtimeConnectionDetails struct {
	UUID                    uuid.UUID
	Location                string
	TunnelIsRemotelyManaged bool
}

func (d *runtimeConnectionDetails) unmarshalCapnproto(s proto.ConnectionDetails) error {
	uuidBytes, err := s.Uuid()
	if err != nil {
		return err
	}
	tunnelID, err := uuid.FromBytes(uuidBytes)
	if err != nil {
		return err
	}
	location, err := s.LocationName()
	if err != nil {
		return err
	}
	d.UUID = tunnelID
	d.Location = location
	d.TunnelIsRemotelyManaged = s.TunnelIsRemotelyManaged()
	return nil
}

type runtimeRegisterUDPSessionResponse struct {
	Err   error
	Spans []byte
}

func (r *runtimeRegisterUDPSessionResponse) toPogs() *tunnelpogs.RegisterUdpSessionResponse {
	if r == nil {
		return nil
	}
	return &tunnelpogs.RegisterUdpSessionResponse{
		Err:   r.Err,
		Spans: append([]byte(nil), r.Spans...),
	}
}

type runtimeUpdateConfigurationResponse struct {
	LastAppliedVersion int32 `json:"lastAppliedVersion"`
	Err                error `json:"err"`
}

func (r *runtimeUpdateConfigurationResponse) toPogs() *tunnelpogs.UpdateConfigurationResponse {
	if r == nil {
		return nil
	}
	return &tunnelpogs.UpdateConfigurationResponse{
		LastAppliedVersion: r.LastAppliedVersion,
		Err:                r.Err,
	}
}

type runtimeRegistrationServerClient interface {
	RegisterConnection(context.Context, runtimeTunnelAuth, [16]byte, uint8, *runtimeConnectionOptions) (*runtimeConnectionDetails, error)
	SendLocalConfiguration(context.Context, []byte) error
	UnregisterConnection(context.Context) error
	Close() error
}

type runtimeRegistrationServerPogsClient struct {
	client capnp.Client
	conn   *rpc.Conn
}

func (c runtimeRegistrationServerPogsClient) RegisterConnection(ctx context.Context, auth runtimeTunnelAuth, tunnelID [16]byte, connIndex uint8, options *runtimeConnectionOptions) (*runtimeConnectionDetails, error) {
	client := proto.TunnelServer{Client: c.client}
	promise := client.RegisterConnection(ctx, func(params proto.RegistrationServer_registerConnection_Params) error {
		tunnelAuth, err := params.NewAuth()
		if err != nil {
			return err
		}
		if err := auth.marshalCapnproto(tunnelAuth); err != nil {
			return err
		}
		if err := params.SetAuth(tunnelAuth); err != nil {
			return err
		}
		if err := params.SetTunnelId(tunnelID[:]); err != nil {
			return err
		}
		params.SetConnIndex(connIndex)
		connectionOptions, err := params.NewOptions()
		if err != nil {
			return err
		}
		return options.marshalCapnproto(connectionOptions)
	})
	response, err := promise.Result().Struct()
	if err != nil {
		return nil, wrapRuntimeRPCError(err)
	}
	result := response.Result()
	switch result.Which() {
	case proto.ConnectionResponse_result_Which_error:
		resultError, err := result.Error()
		if err != nil {
			return nil, wrapRuntimeRPCError(err)
		}
		cause, err := resultError.Cause()
		if err != nil {
			return nil, wrapRuntimeRPCError(err)
		}
		err = errors.New(cause)
		if resultError.ShouldRetry() {
			err = runtimeRetryErrorAfter(err, time.Duration(resultError.RetryAfter()))
		}
		return nil, err
	case proto.ConnectionResponse_result_Which_connectionDetails:
		connDetails, err := result.ConnectionDetails()
		if err != nil {
			return nil, wrapRuntimeRPCError(err)
		}
		details := new(runtimeConnectionDetails)
		if err := details.unmarshalCapnproto(connDetails); err != nil {
			return nil, wrapRuntimeRPCError(err)
		}
		return details, nil
	default:
		return nil, newRuntimeRPCError("unknown result which %d", result.Which())
	}
}

func (c runtimeRegistrationServerPogsClient) SendLocalConfiguration(ctx context.Context, config []byte) error {
	client := proto.TunnelServer{Client: c.client}
	promise := client.UpdateLocalConfiguration(ctx, func(params proto.RegistrationServer_updateLocalConfiguration_Params) error {
		return params.SetConfig(config)
	})
	_, err := promise.Struct()
	return wrapRuntimeRPCError(err)
}

func (c runtimeRegistrationServerPogsClient) UnregisterConnection(ctx context.Context) error {
	client := proto.TunnelServer{Client: c.client}
	promise := client.UnregisterConnection(ctx, func(params proto.RegistrationServer_unregisterConnection_Params) error {
		return nil
	})
	_, err := promise.Struct()
	return wrapRuntimeRPCError(err)
}

func (c runtimeRegistrationServerPogsClient) Close() error {
	c.client.Close()
	return c.conn.Close()
}

type runtimeRetryableError struct {
	err   error
	delay time.Duration
}

func (e *runtimeRetryableError) Error() string {
	return e.err.Error()
}

func (e *runtimeRetryableError) Unwrap() error {
	return e.err
}

func runtimeRetryErrorAfter(err error, delay time.Duration) *runtimeRetryableError {
	return &runtimeRetryableError{err: err, delay: delay}
}

type runtimeRPCError struct {
	err error
}

func (e *runtimeRPCError) Error() string {
	return e.err.Error()
}

func (e *runtimeRPCError) Unwrap() error {
	return e.err
}

func wrapRuntimeRPCError(err error) error {
	if err == nil {
		return nil
	}
	return &runtimeRPCError{err: err}
}

func newRuntimeRPCError(format string, args ...any) *runtimeRPCError {
	return &runtimeRPCError{err: fmt.Errorf(format, args...)}
}

type runtimeConnectionType uint16

const (
	runtimeConnectionTypeHTTP runtimeConnectionType = iota
	runtimeConnectionTypeWebsocket
	runtimeConnectionTypeTCP
)

func (c runtimeConnectionType) String() string {
	switch c {
	case runtimeConnectionTypeHTTP:
		return "http"
	case runtimeConnectionTypeWebsocket:
		return "ws"
	case runtimeConnectionTypeTCP:
		return "tcp"
	default:
		panic(fmt.Sprintf("invalid runtimeConnectionType: %d", c))
	}
}

type runtimeConnectRequest struct {
	Dest     string                `capnp:"dest"`
	Type     runtimeConnectionType `capnp:"type"`
	Metadata []runtimeMetadata     `capnp:"metadata"`
}

func (r *runtimeConnectRequest) MetadataMap() map[string]string {
	metadataMap := make(map[string]string)
	for _, metadata := range r.Metadata {
		metadataMap[metadata.Key] = metadata.Val
	}
	return metadataMap
}

func (r *runtimeConnectRequest) FromPogs(msg *capnp.Message) error {
	metadata, err := proto.ReadRootConnectRequest(msg)
	if err != nil {
		return err
	}
	return capnppogs.Extract(r, proto.ConnectRequest_TypeID, metadata.Struct)
}

func (r *runtimeConnectRequest) ToPogs() (*capnp.Message, error) {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, err
	}

	root, err := proto.NewRootConnectRequest(seg)
	if err != nil {
		return nil, err
	}

	if err := capnppogs.Insert(proto.ConnectRequest_TypeID, root.Struct, r); err != nil {
		return nil, err
	}

	return msg, nil
}

type runtimeMetadata struct {
	Key string `capnp:"key"`
	Val string `capnp:"val"`
}

type runtimeConnectResponse struct {
	Error    string            `capnp:"error"`
	Metadata []runtimeMetadata `capnp:"metadata"`
}

func (r *runtimeConnectResponse) FromPogs(msg *capnp.Message) error {
	metadata, err := proto.ReadRootConnectResponse(msg)
	if err != nil {
		return err
	}
	return capnppogs.Extract(r, proto.ConnectResponse_TypeID, metadata.Struct)
}

func (r *runtimeConnectResponse) ToPogs() (*capnp.Message, error) {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, err
	}

	root, err := proto.NewRootConnectResponse(seg)
	if err != nil {
		return nil, err
	}

	if err := capnppogs.Insert(proto.ConnectResponse_TypeID, root.Struct, r); err != nil {
		return nil, err
	}

	return msg, nil
}

var runtimeErrorFlowConnectRateLimitedMetadata = runtimeMetadata{
	Key: "FlowConnectRateLimited",
	Val: "true",
}

type runtimeSessionManagerPogsAdapter struct {
	manager runtimeSessionManager
}

func (a runtimeSessionManagerPogsAdapter) RegisterUdpSession(ctx context.Context, sessionID uuid.UUID, dstIP net.IP, dstPort uint16, closeAfterIdleHint time.Duration, traceContext string) (*tunnelpogs.RegisterUdpSessionResponse, error) {
	response, err := a.manager.RegisterUdpSession(ctx, sessionID, dstIP, dstPort, closeAfterIdleHint, traceContext)
	if response == nil {
		return nil, err
	}
	return response.toPogs(), err
}

func (a runtimeSessionManagerPogsAdapter) UnregisterUdpSession(ctx context.Context, sessionID uuid.UUID, message string) error {
	return a.manager.UnregisterUdpSession(ctx, sessionID, message)
}

type runtimeConfigurationManagerPogsAdapter struct {
	manager runtimeConfigurationManager
}

func (a runtimeConfigurationManagerPogsAdapter) UpdateConfiguration(ctx context.Context, version int32, config []byte) *tunnelpogs.UpdateConfigurationResponse {
	response := a.manager.UpdateConfiguration(ctx, version, config)
	return response.toPogs()
}

func newRuntimeCloudflaredServerClient(sessionManager runtimeSessionManager, configManager runtimeConfigurationManager) capnp.Client {
	return tunnelpogs.CloudflaredServer_ServerToClient(
		runtimeSessionManagerPogsAdapter{manager: sessionManager},
		runtimeConfigurationManagerPogsAdapter{manager: configManager},
	).Client
}

func newRuntimeRegistrationServerClient(client capnp.Client, conn *rpc.Conn) runtimeRegistrationServerClient {
	return runtimeRegistrationServerPogsClient{
		client: client,
		conn:   conn,
	}
}
