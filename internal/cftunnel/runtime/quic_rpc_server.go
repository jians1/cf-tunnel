package runtime

import (
	"context"
	"fmt"
	"io"
	"time"

	capnp "zombiezen.com/go/capnproto2"
)

type protocolSignature [6]byte

var (
	dataStreamProtocolSignature = protocolSignature{0x0A, 0x36, 0xCD, 0x12, 0xA1, 0x3E}
	rpcStreamProtocolSignature  = protocolSignature{0x52, 0xBB, 0x82, 0x5C, 0xDB, 0x65}
)

const (
	protocolV1            = "01"
	protocolVersionLength = 2
)

type runtimeRequestServerStream struct {
	io.ReadWriteCloser
}

func (rss *runtimeRequestServerStream) ReadConnectRequestData() (*runtimeConnectRequest, error) {
	if _, err := readVersion(rss); err != nil {
		return nil, err
	}

	msg, err := capnp.NewDecoder(rss).Decode()
	if err != nil {
		return nil, err
	}

	req := &runtimeConnectRequest{}
	if err := req.FromPogs(msg); err != nil {
		return nil, err
	}
	return req, nil
}

func (rss *runtimeRequestServerStream) WriteConnectResponseData(respErr error, metadata ...runtimeMetadata) error {
	resp := &runtimeConnectResponse{Metadata: metadata}
	if respErr != nil {
		resp.Error = respErr.Error()
	}

	msg, err := resp.ToPogs()
	if err != nil {
		return err
	}
	if err := writeDataStreamPreamble(rss); err != nil {
		return err
	}
	return capnp.NewEncoder(rss).Encode(msg)
}

type runtimeHandleRequestFunc func(ctx context.Context, stream *runtimeRequestServerStream) error

type runtimeCloudflaredServer struct {
	handleRequest   runtimeHandleRequestFunc
	sessionManager  runtimeSessionManager
	configManager   runtimeConfigurationManager
	responseTimeout time.Duration
}

func newRuntimeCloudflaredServer(handleRequest runtimeHandleRequestFunc, sessionManager runtimeSessionManager, configManager runtimeConfigurationManager, responseTimeout time.Duration) *runtimeCloudflaredServer {
	return &runtimeCloudflaredServer{
		handleRequest:   handleRequest,
		sessionManager:  sessionManager,
		configManager:   configManager,
		responseTimeout: responseTimeout,
	}
}

func (s *runtimeCloudflaredServer) Serve(ctx context.Context, stream io.ReadWriteCloser) error {
	signature, err := determineProtocol(stream)
	if err != nil {
		return err
	}

	switch signature {
	case dataStreamProtocolSignature:
		return s.handleRequest(ctx, &runtimeRequestServerStream{stream})
	case rpcStreamProtocolSignature:
		return s.handleRPC(ctx, stream)
	default:
		return fmt.Errorf("unknown protocol %v", signature)
	}
}

func (s *runtimeCloudflaredServer) handleRPC(ctx context.Context, stream io.ReadWriteCloser) error {
	ctx, cancel := context.WithTimeout(ctx, s.responseTimeout)
	defer cancel()

	transport := safeTransport(stream)
	defer transport.Close()

	rpcConn := newServerConn(transport, newRuntimeCloudflaredServerClient(s.sessionManager, s.configManager))
	defer rpcConn.Close()

	select {
	case <-rpcConn.Done():
	case <-ctx.Done():
	}
	return nil
}

func determineProtocol(stream io.Reader) (protocolSignature, error) {
	signature, err := readSignature(stream)
	if err != nil {
		return protocolSignature{}, err
	}
	switch signature {
	case dataStreamProtocolSignature:
		return dataStreamProtocolSignature, nil
	case rpcStreamProtocolSignature:
		return rpcStreamProtocolSignature, nil
	default:
		return protocolSignature{}, fmt.Errorf("unknown signature %v", signature)
	}
}

func writeDataStreamPreamble(stream io.Writer) error {
	if err := writeSignature(stream, dataStreamProtocolSignature); err != nil {
		return err
	}
	return writeVersion(stream)
}

func writeVersion(stream io.Writer) error {
	_, err := stream.Write([]byte(protocolV1)[:protocolVersionLength])
	return err
}

func readVersion(stream io.Reader) (string, error) {
	version := make([]byte, protocolVersionLength)
	_, err := stream.Read(version)
	return string(version), err
}

func readSignature(stream io.Reader) (protocolSignature, error) {
	var signature protocolSignature
	if _, err := io.ReadFull(stream, signature[:]); err != nil {
		return protocolSignature{}, err
	}
	return signature, nil
}

func writeSignature(stream io.Writer, signature protocolSignature) error {
	_, err := stream.Write(signature[:])
	return err
}
