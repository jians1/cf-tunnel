package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

const maxHAConnections = 256

type bridgeInstance interface {
	Run(context.Context) error
}

type bridgeInstanceFactory func(Session, *slog.Logger, InstanceOptions) (bridgeInstance, error)

type BridgeRunner struct {
	session         Session
	logger          *slog.Logger
	http2Options    HTTP2ServerOptions
	quicOptions     QUICRuntimeOptions
	instanceFactory bridgeInstanceFactory
}

func NewBridgeRunner(session Session, logger *slog.Logger) *BridgeRunner {
	if logger == nil {
		logger = slog.Default()
	}

	return &BridgeRunner{
		session:         session,
		logger:          logger.With("component", "cftunnel-runtime"),
		instanceFactory: defaultBridgeInstanceFactory,
	}
}

func (r *BridgeRunner) SetHTTP2Options(opts HTTP2ServerOptions) {
	r.http2Options = opts
}

func (r *BridgeRunner) SetQUICOptions(opts QUICRuntimeOptions) {
	r.quicOptions = opts
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

	haConnections := normalizeHAConnections(r.session.HAConnections)
	if haConnections == 1 {
		return r.runConnection(ctx, 0)
	}

	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(haConnections)
	for i := 0; i < haConnections; i++ {
		connIndex := uint8(i)
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

func (r *BridgeRunner) runConnection(ctx context.Context, connIndex uint8) error {
	factory := r.instanceFactory
	if factory == nil {
		factory = defaultBridgeInstanceFactory
	}
	instance, err := factory(r.session, r.logger.With("conn_index", connIndex), r.instanceOptions(connIndex))
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

func (r *BridgeRunner) instanceOptions(connIndex uint8) InstanceOptions {
	http2Options := r.http2Options
	http2Options.ConnIndex = connIndex
	if http2Options.DialConfig != nil {
		http2Options.DialConfig = http2Options.DialConfig.Clone()
	}

	quicOptions := r.quicOptions
	quicOptions.ConnIndex = connIndex
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

func defaultBridgeInstanceFactory(session Session, logger *slog.Logger, options InstanceOptions) (bridgeInstance, error) {
	return NewInstanceWithRuntimeOptions(session, logger, options)
}
