package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const maxHAConnections = 256
const defaultHARegistrationInterval = time.Second

const (
	defaultMaxConnRetries = 5
	defaultBackoffBase    = time.Second
	defaultBackoffCap     = 15 * time.Second
)

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
	maxConnRetries       int
	backoffBase          time.Duration
	backoffCap           time.Duration
	edgePool             *edgeAddressPool
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
		maxConnRetries:       defaultMaxConnRetries,
		backoffBase:          defaultBackoffBase,
		backoffCap:           defaultBackoffCap,
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

// ensureEdgePool builds the shared edge address pool from the configured
// Cloudflare edge address provider for the active protocol. The pool hands each
// HA connection a distinct edge server so a second connection is not rejected
// with EDUPCONN, and lets rotate pick a fresh server on reconnect. Custom
// providers (used in tests) leave the pool nil and keep the per-connection
// provider path unchanged.
func (r *BridgeRunner) ensureEdgePool() {
	if r.edgePool != nil {
		return
	}
	var provider EdgeAddressProvider
	switch r.session.Edge.Protocol {
	case edgeProtocolQUIC:
		provider = r.quicOptions.EdgeAddressProvider
	default:
		provider = r.http2Options.EdgeAddressProvider
	}
	cf, ok := provider.(*CloudflareEdgeAddressProvider)
	if !ok {
		return
	}
	r.edgePool = newCloudflareEdgeAddressPool(cf)
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
	r.ensureEdgePool()

	haConnections := normalizeHAConnections(r.session.HAConnections)
	if haConnections == 1 {
		return r.runConnectionWithRetry(ctx, 0, nil)
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
		err := r.runConnectionWithRetry(groupCtx, 0, &connectedSignalFuse{
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
			if err := r.runConnectionWithRetry(groupCtx, connIndex, nil); err != nil {
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

// nonRetryableError marks a failure that should not trigger a reconnect,
// such as a configuration or instance-build error. Transport failures that
// happen after (or during) dialing are retryable so a single HA connection
// can recover without tearing down the whole process.
type nonRetryableError struct {
	err error
}

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

// runConnectionWithRetry keeps a single HA connection alive across transient
// edge failures (EDUPCONN, "connection with edge closed", dial errors). Each
// attempt rotates to a different edge address and backs off, mirroring the
// upstream cloudflared supervisor. It gives up only after maxConnRetries
// consecutive failures without a successful connection, at which point the
// error propagates and the process exits (systemd then restarts).
func (r *BridgeRunner) runConnectionWithRetry(ctx context.Context, connIndex uint8, extraFuse ConnectedFuse) error {
	attempts := 0
	for {
		tracker := &connectTrackingFuse{delegate: extraFuse}
		err := r.runConnectionWithConnectedFuse(ctx, connIndex, tracker)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err == nil {
			return nil
		}
		var nonRetryable *nonRetryableError
		if errors.As(err, &nonRetryable) {
			return nonRetryable.err
		}
		if tracker.didConnect() {
			// A connection that served before dropping resets the budget,
			// so a long-lived tunnel is not penalized for an old blip.
			attempts = 0
		}
		attempts++
		if attempts > r.maxConnRetries {
			return fmt.Errorf("connection %d exhausted %d retries: %w", connIndex, r.maxConnRetries, err)
		}
		r.logger.Warn(
			"edge connection failed, retrying",
			"conn_index", connIndex,
			"attempt", attempts,
			"error", err,
		)
		r.rotateEdgeAddress(connIndex)
		if waitErr := r.waitBackoff(ctx, attempts); waitErr != nil {
			return waitErr
		}
	}
}

// waitBackoff sleeps for an exponentially increasing duration capped at
// backoffCap, returning early if the context is cancelled.
func (r *BridgeRunner) waitBackoff(ctx context.Context, attempt int) error {
	delay := r.backoffDuration(attempt)
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *BridgeRunner) backoffDuration(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := r.backoffBase
	if base <= 0 {
		return 0
	}
	delay := base
	for i := 1; i < attempt; i++ {
		delay *= 2
		if r.backoffCap > 0 && delay >= r.backoffCap {
			return r.backoffCap
		}
	}
	if r.backoffCap > 0 && delay > r.backoffCap {
		return r.backoffCap
	}
	return delay
}

// rotateEdgeAddress asks the edge pool (if configured) to hand this connection
// a different edge address before the next attempt. It is a no-op until the
// shared pool is wired in.
func (r *BridgeRunner) rotateEdgeAddress(connIndex uint8) {
	if r.edgePool != nil {
		r.edgePool.rotate(connIndex)
	}
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
		return &nonRetryableError{err: fmt.Errorf("build runtime instance: %w", err)}
	}
	runtimeInstance, ok := instance.(*Instance)
	if ok && runtimeInstance.HTTP2Server != nil {
		r.logger.Debug("http2 runtime server composed")
		if r.http2Options.LocalEdgeDriver {
			driver, err := NewHTTP2LocalEdgeDriver(runtimeInstance.HTTP2Server)
			if err != nil {
				return &nonRetryableError{err: err}
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

// connectTrackingFuse wraps a delegate fuse and records whether Connected was
// called during a single connection attempt. The retry loop uses didConnect to
// reset its backoff budget after an attempt that actually served traffic.
type connectTrackingFuse struct {
	delegate  ConnectedFuse
	mu        sync.Mutex
	connected bool
}

func (f *connectTrackingFuse) Connected() {
	f.mu.Lock()
	f.connected = true
	f.mu.Unlock()
	if f.delegate != nil {
		f.delegate.Connected()
	}
}

func (f *connectTrackingFuse) IsConnected() bool {
	if f.delegate != nil {
		return f.delegate.IsConnected()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *connectTrackingFuse) didConnect() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}
