package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/cloudflare/cloudflared/tunnelrpc/proto"
	"github.com/google/uuid"
	capnp "zombiezen.com/go/capnproto2"
	"zombiezen.com/go/capnproto2/rpc"
	"zombiezen.com/go/capnproto2/server"
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
	clientInfo, err := s.NewClient()
	if err != nil {
		return err
	}
	if err := clientInfo.SetClientId(o.Client.ClientID); err != nil {
		return err
	}
	features, err := clientInfo.NewFeatures(int32(len(o.Client.Features)))
	if err != nil {
		return err
	}
	for i, feature := range o.Client.Features {
		if err := features.Set(i, feature); err != nil {
			return err
		}
	}
	if err := clientInfo.SetVersion(o.Client.Version); err != nil {
		return err
	}
	if err := clientInfo.SetArch(o.Client.Arch); err != nil {
		return err
	}
	if err := s.SetClient(clientInfo); err != nil {
		return err
	}
	if err := s.SetOriginLocalIp([]byte(o.OriginLocalIP)); err != nil {
		return err
	}
	s.SetReplaceExisting(o.ReplaceExisting)
	s.SetCompressionQuality(o.CompressionQuality)
	s.SetNumPreviousAttempts(o.NumPreviousAttempts)
	return nil
}

type runtimeTunnelAuth struct {
	AccountTag   string
	TunnelSecret []byte
}

func (a runtimeTunnelAuth) marshalCapnproto(s proto.TunnelAuth) error {
	if err := s.SetAccountTag(a.AccountTag); err != nil {
		return err
	}
	return s.SetTunnelSecret(a.TunnelSecret)
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

func (r *runtimeRegisterUDPSessionResponse) marshalCapnproto(s proto.RegisterUdpSessionResponse) error {
	if r.Err != nil {
		return s.SetErr(r.Err.Error())
	}
	return s.SetSpans(r.Spans)
}

type runtimeUpdateConfigurationResponse struct {
	LastAppliedVersion int32 `json:"lastAppliedVersion"`
	Err                error `json:"err"`
}

func (r *runtimeUpdateConfigurationResponse) marshalCapnproto(s proto.UpdateConfigurationResponse) error {
	s.SetLatestAppliedVersion(r.LastAppliedVersion)
	if r.Err != nil {
		return s.SetErr(r.Err.Error())
	}
	return nil
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
	req, err := proto.ReadRootConnectRequest(msg)
	if err != nil {
		return err
	}

	dest, err := req.Dest()
	if err != nil {
		return err
	}
	metadataList, err := req.Metadata()
	if err != nil {
		return err
	}

	metadata := make([]runtimeMetadata, 0, metadataList.Len())
	for i := 0; i < metadataList.Len(); i++ {
		item := metadataList.At(i)
		key, err := item.Key()
		if err != nil {
			return err
		}
		val, err := item.Val()
		if err != nil {
			return err
		}
		metadata = append(metadata, runtimeMetadata{Key: key, Val: val})
	}

	r.Dest = dest
	r.Type = runtimeConnectionType(req.Type())
	r.Metadata = metadata
	return nil
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

	if err := root.SetDest(r.Dest); err != nil {
		return nil, err
	}
	root.SetType(proto.ConnectionType(r.Type))
	metadata, err := root.NewMetadata(int32(len(r.Metadata)))
	if err != nil {
		return nil, err
	}
	for i, entry := range r.Metadata {
		item := metadata.At(i)
		if err := item.SetKey(entry.Key); err != nil {
			return nil, err
		}
		if err := item.SetVal(entry.Val); err != nil {
			return nil, err
		}
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
	resp, err := proto.ReadRootConnectResponse(msg)
	if err != nil {
		return err
	}

	errText, err := resp.Error()
	if err != nil {
		return err
	}
	metadataList, err := resp.Metadata()
	if err != nil {
		return err
	}

	metadata := make([]runtimeMetadata, 0, metadataList.Len())
	for i := 0; i < metadataList.Len(); i++ {
		item := metadataList.At(i)
		key, err := item.Key()
		if err != nil {
			return err
		}
		val, err := item.Val()
		if err != nil {
			return err
		}
		metadata = append(metadata, runtimeMetadata{Key: key, Val: val})
	}

	r.Error = errText
	r.Metadata = metadata
	return nil
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

	if err := root.SetError(r.Error); err != nil {
		return nil, err
	}
	metadata, err := root.NewMetadata(int32(len(r.Metadata)))
	if err != nil {
		return nil, err
	}
	for i, entry := range r.Metadata {
		item := metadata.At(i)
		if err := item.SetKey(entry.Key); err != nil {
			return nil, err
		}
		if err := item.SetVal(entry.Val); err != nil {
			return nil, err
		}
	}

	return msg, nil
}

var runtimeErrorFlowConnectRateLimitedMetadata = runtimeMetadata{
	Key: "FlowConnectRateLimited",
	Val: "true",
}

type runtimeCloudflaredServerClient struct {
	sessionManager runtimeSessionManager
	configManager  runtimeConfigurationManager
}

func (c runtimeCloudflaredServerClient) RegisterUdpSession(p proto.SessionManager_registerUdpSession) error {
	server.Ack(p.Options)

	sessionIDRaw, err := p.Params.SessionId()
	if err != nil {
		return err
	}
	sessionID, err := uuid.FromBytes(sessionIDRaw)
	if err != nil {
		return err
	}

	dstIPRaw, err := p.Params.DstIp()
	if err != nil {
		return err
	}
	dstIP := net.IP(dstIPRaw)
	if dstIP == nil {
		return fmt.Errorf("%v is not valid IP", dstIPRaw)
	}

	traceContext, err := p.Params.TraceContext()
	if err != nil {
		return err
	}

	response, registrationErr := c.sessionManager.RegisterUdpSession(
		p.Ctx,
		sessionID,
		dstIP,
		p.Params.DstPort(),
		time.Duration(p.Params.CloseAfterIdleHint()),
		traceContext,
	)
	if registrationErr != nil {
		if response == nil {
			response = &runtimeRegisterUDPSessionResponse{}
		}
		response.Err = registrationErr
	}
	if response == nil {
		return errors.New("runtime UDP session manager returned nil response")
	}

	result, err := p.Results.NewResult()
	if err != nil {
		return err
	}
	return response.marshalCapnproto(result)
}

func (c runtimeCloudflaredServerClient) UnregisterUdpSession(p proto.SessionManager_unregisterUdpSession) error {
	server.Ack(p.Options)

	sessionIDRaw, err := p.Params.SessionId()
	if err != nil {
		return err
	}
	sessionID, err := uuid.FromBytes(sessionIDRaw)
	if err != nil {
		return err
	}
	message, err := p.Params.Message()
	if err != nil {
		return err
	}
	return c.sessionManager.UnregisterUdpSession(p.Ctx, sessionID, message)
}

func (c runtimeCloudflaredServerClient) UpdateConfiguration(p proto.ConfigurationManager_updateConfiguration) error {
	server.Ack(p.Options)

	config, err := p.Params.Config()
	if err != nil {
		return err
	}
	response := c.configManager.UpdateConfiguration(p.Ctx, p.Params.Version(), config)
	if response == nil {
		return errors.New("runtime configuration manager returned nil response")
	}

	result, err := p.Results.NewResult()
	if err != nil {
		return err
	}
	return response.marshalCapnproto(result)
}

func newRuntimeCloudflaredServerClient(sessionManager runtimeSessionManager, configManager runtimeConfigurationManager) capnp.Client {
	return proto.CloudflaredServer_ServerToClient(runtimeCloudflaredServerClient{
		sessionManager: sessionManager,
		configManager:  configManager,
	}).Client
}

func newRuntimeRegistrationServerClient(client capnp.Client, conn *rpc.Conn) runtimeRegistrationServerClient {
	return runtimeRegistrationServerPogsClient{
		client: client,
		conn:   conn,
	}
}
