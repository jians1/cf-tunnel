package runtime

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/netip"
	"time"

	cfdconnection "github.com/cloudflare/cloudflared/connection"
	"github.com/cloudflare/cloudflared/connection/dialopts"
	cfdquic "github.com/cloudflare/cloudflared/quic"
	tunnelpogs "github.com/cloudflare/cloudflared/tunnelrpc/pogs"
	quicgo "github.com/quic-go/quic-go"
)

type QUICRuntime struct {
	tunnelConn interface {
		Serve(context.Context) error
	}
	quicConn   quicConnectionCloser
	serverConn *quicgo.Listener
	udpConn    *net.UDPConn
}

type quicConnectionCloser interface {
	CloseWithError(quicgo.ApplicationErrorCode, string) error
}

var errNilEntropyReader = errors.New("nil entropy reader")
var errMissingQUICDialConfig = errors.New("missing quic edge dial config")

const (
	defaultQUICConnectionReceiveWindow uint64 = 30 * (1 << 20)
	defaultQUICStreamReceiveWindow     uint64 = 6 * (1 << 20)
)

func NewQUICRuntime(session Session, logger *slog.Logger) (*QUICRuntime, error) {
	return NewQUICRuntimeWithOptions(session, logger, QUICRuntimeOptions{})
}

func NewQUICRuntimeWithOptions(session Session, logger *slog.Logger, options QUICRuntimeOptions) (*QUICRuntime, error) {
	if options.DialConfig != nil {
		return newQUICRuntimeWithEdgeDialConfig(session, logger, options.DialConfig)
	}
	return nil, errMissingQUICDialConfig
}

func newQUICRuntimeWithEdgeDialConfig(session Session, logger *slog.Logger, dialConfig *QUICDialConfig) (*QUICRuntime, error) {
	if dialConfig == nil {
		return nil, fmt.Errorf("nil quic dial config")
	}

	log := newZeroLoggerFromSlog(logger)
	ctx := context.Background()
	prepared, err := PrepareRuntime(session)
	if err != nil {
		return nil, err
	}
	orchestrator, err := NewUpstreamOrchestrator(NewUpstreamOriginProxy(prepared.OriginProxy), session)
	if err != nil {
		return nil, err
	}

	address, err := dialConfig.resolveAddress()
	if err != nil {
		return nil, err
	}
	edgeAddr, err := netip.ParseAddrPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse quic dial address: %w", err)
	}

	quicConfig := newQUICConfig(edgeAddr.Addr().Is4())
	dialCtx := ctx
	cancel := func() {}
	if dialConfig.Timeout > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, dialConfig.Timeout)
	}
	defer cancel()

	conn, err := cfdconnection.DialQuic(
		dialCtx,
		quicConfig,
		dialConfig.TLSConfig.Clone(),
		edgeAddr,
		nil,
		0,
		&log,
		dialopts.DialOpts{},
	)
	if err != nil {
		return nil, err
	}

	connOptions, err := newRuntimeConnectionOptions()
	if err != nil {
		_ = conn.CloseWithError(0, "")
		return nil, fmt.Errorf("build quic connection options: %w", err)
	}
	binding, err := NewUpstreamAdapter().Bind(session)
	if err != nil {
		_ = conn.CloseWithError(0, "")
		return nil, err
	}
	controlStreamHandler := NewControlStream(runtimeControlStreamOptions{
		ConnectedFuse:      noopConnectedFuse{},
		TunnelProperties:   binding.TunnelProperties,
		ConnIndex:          0,
		EdgeAddress:        net.IP(edgeAddr.Addr().AsSlice()),
		RegisterClientFunc: nil,
		RegisterTimeout:    time.Second,
		GracefulShutdownC:  nil,
		GracePeriod:        time.Second,
	})

	tunnelConn := NewRuntimeQUICConnection(
		conn,
		orchestrator,
		newNoopDatagramSessionHandler(),
		controlStreamHandler,
		connOptions,
		0,
		15*time.Second,
		0,
		time.Second,
		&log,
	)

	return &QUICRuntime{
		tunnelConn: tunnelConn,
		quicConn:   conn,
	}, nil
}

func closeQUICRuntimeStartupResources(conn quicConnectionCloser, listener io.Closer, udpConn io.Closer) error {
	var err error
	if conn != nil {
		err = errors.Join(err, conn.CloseWithError(0, ""))
	}
	if listener != nil {
		err = errors.Join(err, listener.Close())
	}
	if udpConn != nil {
		err = errors.Join(err, udpConn.Close())
	}
	return err
}

func newQUICConfig(edgeIsIPv4 bool) *quicgo.Config {
	initialPacketSize := uint16(1252)
	if edgeIsIPv4 {
		initialPacketSize = 1232
	}
	return &quicgo.Config{
		HandshakeIdleTimeout:  cfdquic.HandshakeIdleTimeout,
		MaxIdleTimeout:        cfdquic.MaxIdleTimeout,
		KeepAlivePeriod:       cfdquic.MaxIdlePingPeriod,
		MaxIncomingStreams:    cfdquic.MaxIncomingStreams,
		MaxIncomingUniStreams: cfdquic.MaxIncomingStreams,
		EnableDatagrams:       true,
		MaxConnectionReceiveWindow: defaultQUICConnectionReceiveWindow,
		MaxStreamReceiveWindow:     defaultQUICStreamReceiveWindow,
		InitialPacketSize:     initialPacketSize,
	}
}

func (r *QUICRuntime) Run(ctx context.Context) error {
	if r == nil || r.tunnelConn == nil {
		return fmt.Errorf("nil quic runtime")
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.tunnelConn.Serve(ctx)
	}()

	select {
	case err := <-errCh:
		_ = r.Close()
		return err
	case <-ctx.Done():
		_ = r.Close()
		return ctx.Err()
	}
}

func (r *QUICRuntime) Close() error {
	if r == nil {
		return nil
	}
	var err error
	if r.quicConn != nil {
		err = errors.Join(err, r.quicConn.CloseWithError(0, ""))
	}
	if r.serverConn != nil {
		err = errors.Join(err, r.serverConn.Close())
	}
	if r.udpConn != nil {
		err = errors.Join(err, r.udpConn.Close())
	}
	return err
}

func generateQUICServerTLSConfig() (*tls.Config, error) {
	return generateQUICServerTLSConfigWithReader(rand.Reader)
}

func generateQUICServerTLSConfigWithReader(random io.Reader) (*tls.Config, error) {
	if random == nil {
		return nil, errNilEntropyReader
	}
	key, err := rsa.GenerateKey(random, 1024)
	if err != nil {
		return nil, fmt.Errorf("generate quic server key: %w", err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(random, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create quic server certificate: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load quic server certificate: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{edgeALPNQUIC},
	}, nil
}

type fakeQUICControlStream struct {
	cfdconnection.ControlStreamHandler
}

func (fakeQUICControlStream) ServeControlStream(ctx context.Context, rw io.ReadWriteCloser, connOptions *tunnelpogs.ConnectionOptions, tunnelConfigGetter TunnelConfigJSONGetter) error {
	<-ctx.Done()
	return nil
}

func (fakeQUICControlStream) IsStopped() bool {
	return true
}
