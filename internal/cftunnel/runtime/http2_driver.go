package runtime

import (
	"context"
	"fmt"
)

type HTTP2LocalEdgeDriver struct {
	server *HTTP2Server
	client *HTTP2Client
}

func NewHTTP2LocalEdgeDriver(server *HTTP2Server) (*HTTP2LocalEdgeDriver, error) {
	if server == nil {
		return nil, fmt.Errorf("nil http2 server")
	}
	return &HTTP2LocalEdgeDriver{
		server: server,
	}, nil
}

func (d *HTTP2LocalEdgeDriver) Run(ctx context.Context) error {
	if d == nil || d.server == nil {
		return fmt.Errorf("nil http2 local edge driver")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.server.Serve(ctx)
	}()

	client, err := NewHTTP2Client(d.server)
	if err != nil {
		_ = d.server.EdgeConn().Close()
		return err
	}
	d.client = client

	select {
	case err := <-errCh:
		_ = d.Close()
		return err
	case <-ctx.Done():
		_ = d.Close()
		<-errCh
		return ctx.Err()
	}
}

func (d *HTTP2LocalEdgeDriver) Close() error {
	if d == nil || d.client == nil {
		return nil
	}
	return d.client.Close()
}
