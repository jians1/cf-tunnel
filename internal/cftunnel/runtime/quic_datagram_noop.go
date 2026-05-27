package runtime

import (
	"context"
	"fmt"
	"net"
	"time"

	tunnelpogs "github.com/cloudflare/cloudflared/tunnelrpc/pogs"
	"github.com/google/uuid"
)

var errDatagramSessionsDisabled = fmt.Errorf("datagram sessions are disabled")

type noopDatagramSessionHandler struct{}

type datagramSessionHandler interface {
	Serve(context.Context) error
	RegisterUdpSession(context.Context, uuid.UUID, net.IP, uint16, time.Duration, string) (*tunnelpogs.RegisterUdpSessionResponse, error)
	UnregisterUdpSession(context.Context, uuid.UUID, string) error
}

func newNoopDatagramSessionHandler() datagramSessionHandler {
	return noopDatagramSessionHandler{}
}

func (noopDatagramSessionHandler) Serve(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (noopDatagramSessionHandler) RegisterUdpSession(context.Context, uuid.UUID, net.IP, uint16, time.Duration, string) (*tunnelpogs.RegisterUdpSessionResponse, error) {
	return nil, errDatagramSessionsDisabled
}

func (noopDatagramSessionHandler) UnregisterUdpSession(context.Context, uuid.UUID, string) error {
	return errDatagramSessionsDisabled
}
