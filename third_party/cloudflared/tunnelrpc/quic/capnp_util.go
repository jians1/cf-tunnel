package quic

import (
	"context"
	"io"
	"time"

	"github.com/pkg/errors"
	capnp "zombiezen.com/go/capnproto2"
	"zombiezen.com/go/capnproto2/rpc"
)

const (
	defaultSleepBetweenTemporaryError = 500 * time.Millisecond
	defaultMaxRetries                 = 3
)

type readWriterSafeTemporaryErrorCloser struct {
	io.ReadWriteCloser
	retries             int
	sleepBetweenRetries time.Duration
	maxRetries          int
}

func (r *readWriterSafeTemporaryErrorCloser) Read(p []byte) (n int, err error) {
	n, err = r.ReadWriteCloser.Read(p)
	if n == 0 && err != nil && isTemporaryError(err) {
		if r.retries >= r.maxRetries {
			return 0, errors.Wrap(err, "failed read from capnproto ReaderWriter after multiple temporary errors")
		}
		r.retries++
		time.Sleep(r.sleepBetweenRetries)
	}
	if err == nil {
		r.retries = 0
	}
	return n, err
}

func safeTransport(rw io.ReadWriteCloser) rpc.Transport {
	return rpc.StreamTransport(&readWriterSafeTemporaryErrorCloser{
		ReadWriteCloser:     rw,
		maxRetries:          defaultMaxRetries,
		sleepBetweenRetries: defaultSleepBetweenTemporaryError,
	})
}

func isTemporaryError(e error) bool {
	type temp interface{ Temporary() bool }
	t, ok := e.(temp)
	return ok && t.Temporary()
}

type noopCapnpLogger struct{}

func (noopCapnpLogger) Infof(ctx context.Context, format string, args ...interface{})  {}
func (noopCapnpLogger) Errorf(ctx context.Context, format string, args ...interface{}) {}

func newClientConn(transport rpc.Transport) *rpc.Conn {
	return rpc.NewConn(transport, rpc.ConnLog(noopCapnpLogger{}))
}

func newServerConn(transport rpc.Transport, client capnp.Client) *rpc.Conn {
	return rpc.NewConn(transport, rpc.MainInterface(client), rpc.ConnLog(noopCapnpLogger{}))
}
