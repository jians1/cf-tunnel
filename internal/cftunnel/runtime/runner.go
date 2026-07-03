package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const maxHAConnections = 256
const defaultHARegistrationInterval = time.Second

type bridgeInstance interface {
	Run(context.Context) error
}

type bridgeInstanceFactory func(Session, *slog.Logger, InstanceOptions) (bridgeInstance, error)

type BridgeRunner struct {
	session              Session
	logger               *slog.Logger
	http2Options         HTTP2ServerOptions
	quicOptions          QUICRuntimeOptions
	connectorID          []byte
	registrationInterval time.Duration
	instanceFactory      bridgeInstanceFactory
}

func NewBridgeRunner(session Session, logger *slog.Logger) *BridgeRunner {
	if logger == nil {
		logger = slog.Default()
	}

	return &BridgeRunner{
		session:              session,
		logger:               logger.With("component", "cftunnel-runtime"),
		registrationInterval: defaultHARegistrationInterval,
		instanceFactory:      defaultBridgeInstanceFactory,
	}
}

func (r *BridgeRunner) SetHTTP2Options(opts HTTP2ServerOptions) {
	r.http2Options = opts
}

func (r *BridgeRunner) SetQUICOptions(opts QUICRuntimeOptions) {
	r.quicOptions = opts
}

func (r *BridgeRunner) ensureConnectorID() error {
	if len(r.connectorID) == runtimeConnectorIDLength {
		return nil
	}
	connectorID, err := newRuntimeConnectorID()
	if err != nil {
		return err
	}
	r.connectorID = connectorID
	return nil
}

func (r *BridgeRunner) Run(ctx context.Context) error {
	r.logger.Debug(
		"cftunnel runtime bridge prepared",
		"tunnel_id", r.session.TunnelID,
		"hostname", r.session.Hostname,
		"public_url", r.session.PublicURL,
		"edge_protocol", r.session.Edge.Protocol,
		"origin_url", r.session.Origin.URL,
		"origin_protocol", r.session.Origin.Protocol,
		"origin_server_name", r.session.Origin.ServerName,
		"origin_insecure_skip_verify", r.session.Origin.InsecureSkipVerify,
		"origin_websocket_upgrade_mode", r.session.Origin.WebsocketUpgradeMode,
		"ha_connections", r.session.HAConnections,
	)

	if err := r.ensureConnectorID(); err != nil {
		return err
	}

	haConnections := normalizeHAConnections(r.session.HAConnections)
	if haConnections == 1 {
		return r.runConnection(ctx, 0)
	}

	return r.runHAConnections(ctx, haConnections)
}

func (r *BridgeRunner) runHAConnections(ctx context.Context, haConnections int) error {
	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	firstConnected := make(chan struct{})
	firstResult := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := r.runConnectionWithConnectedFuse(groupCtx, 0, &connectedSignalFuse{
			delegate:  r.activeConnectedFuse(),
			connected: firstConnected,
		})
		firstResult <- err
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		cancel()
		wg.Wait()
		return ctx.Err()
	case err := <-firstResult:
		cancel()
		wg.Wait()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	case <-firstConnected:
	}

	startConnection := func(connIndex uint8) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.runConnection(groupCtx, connIndex); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}
	for i := 1; i < haConnections; i++ {
		connIndex := uint8(i)
		startConnection(connIndex)
		if i < haConnections-1 {
			if err := r.waitRegistrationInterval(ctx, errCh); err != nil {
				cancel()
				wg.Wait()
				return err
			}
		}
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		cancel()
		<-done
		return ctx.Err()
	case err := <-errCh:
		cancel()
		<-done
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	case <-done:
		return nil
	}
}

func (r *BridgeRunner) waitRegistrationInterval(ctx context.Context, errCh <-chan error) error {
	if r.registrationInterval <= 0 {
		return nil
	}
	timer := time.NewTimer(r.registrationInterval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	case <-timer.C:
		return nil
	}
}

func (r *BridgeRunner) runConnection(ctx context.Context, connIndex uint8) error {
	return r.runConnectionWithConnectedFuse(ctx, connIndex, nil)
}

func (r *BridgeRunner) runConnectionWithConnectedFuse(ctx context.Context, connIndex uint8, connectedFuse ConnectedFuse) error {
	factory := r.instanceFactory
	if factory == nil {
		factory = defaultBridgeInstanceFactory
	}
	options := r.instanceOptions(connIndex)
	if connectedFuse != nil {
		options.HTTP2.ConnectedFuse = connectedFuse
		options.QUIC.ConnectedFuse = connectedFuse
	}
	instance, err := factory(r.session, r.logger.With("conn_index", connIndex), options)
	if err != nil {
		return fmt.Errorf("build runtime instance: %w", err)
	}
	runtimeInstance, ok := instance.(*Instance)
	if ok && runtimeInstance.HTTP2Server != nil {
		r.logger.Debug("http2 runtime server composed")
		if r.http2Options.LocalEdgeDriver {
			driver, err := NewHTTP2LocalEdgeDriver(runtimeInstance.HTTP2Server)
			if err != nil {
				return err
			}
			return driver.Run(ctx)
		}
	}
	return instance.Run(ctx)
}

func (r *BridgeRunner) activeConnectedFuse() ConnectedFuse {
	if r.http2Options.ConnectedFuse != nil {
		return r.http2Options.ConnectedFuse
	}
	if r.quicOptions.ConnectedFuse != nil {
		return r.quicOptions.ConnectedFuse
	}
	return noopConnectedFuse{}
}

func (r *BridgeRunner) instanceOptions(connIndex uint8) InstanceOptions {
	http2Options := r.http2Options
	http2Options.ConnIndex = connIndex
	http2Options.ConnectorID = append([]byte(nil), r.connectorID...)
	http2Options.EdgeAddressProvider = edgeAddressProviderForConnIndex(http2Options.EdgeAddressProvider, connIndex)
	if http2Options.DialConfig != nil {
		http2Options.DialConfig = http2Options.DialConfig.Clone()
	}

	quicOptions := r.quicOptions
	quicOptions.ConnIndex = connIndex
	quicOptions.ConnectorID = append([]byte(nil), r.connectorID...)
	quicOptions.EdgeAddressProvider = edgeAddressProviderForConnIndex(quicOptions.EdgeAddressProvider, connIndex)
	if quicOptions.DialConfig != nil {
		quicOptions.DialConfig = quicOptions.DialConfig.Clone()
	}

	return InstanceOptions{
		HTTP2: http2Options,
		QUIC:  quicOptions,
	}
}

func normalizeHAConnections(haConnections int) int {
	if haConnections < 1 {
		return 1
	}
	if haConnections > maxHAConnections {
		return maxHAConnections
	}
	return haConnections
}

type connectionIndexedEdgeAddressProvider interface {
	ForConnIndex(uint8) EdgeAddressProvider
}

func edgeAddressProviderForConnIndex(provider EdgeAddressProvider, connIndex uint8) EdgeAddressProvider {
	if indexed, ok := provider.(connectionIndexedEdgeAddressProvider); ok {
		return indexed.ForConnIndex(connIndex)
	}
	return provider
}

func defaultBridgeInstanceFactory(session Session, logger *slog.Logger, options InstanceOptions) (bridgeInstance, error) {
	return NewInstanceWithRuntimeOptions(session, logger, options)
}

type connectedSignalFuse struct {
	delegate  ConnectedFuse
	connected chan struct{}
	once      sync.Once
}

func (f *connectedSignalFuse) Connected() {
	if f.delegate != nil {
		f.delegate.Connected()
	}
	f.once.Do(func() {
		close(f.connected)
	})
}

func (f *connectedSignalFuse) IsConnected() bool {
	if f.delegate == nil {
		return false
	}
	return f.delegate.IsConnected()
}
