package runtime

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	quicgo "github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
)

var quicIdleTimeoutError = quicgo.IdleTimeoutError{}

type runtimeSafeStreamCloser struct {
	lock         sync.Mutex
	stream       quicgo.Stream
	writeTimeout time.Duration
	log          *zerolog.Logger
	closing      atomic.Bool
}

func newRuntimeSafeStreamCloser(stream quicgo.Stream, writeTimeout time.Duration, log *zerolog.Logger) *runtimeSafeStreamCloser {
	return &runtimeSafeStreamCloser{
		stream:       stream,
		writeTimeout: writeTimeout,
		log:          log,
	}
}

func (s *runtimeSafeStreamCloser) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

func (s *runtimeSafeStreamCloser) Write(p []byte) (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.writeTimeout > 0 {
		if err := s.stream.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil && s.log != nil {
			s.log.Err(err).Msg("error setting write deadline for QUIC stream")
		}
	}
	n, err := s.stream.Write(p)
	if err != nil {
		s.handleWriteError(err)
	}
	return n, err
}

func (s *runtimeSafeStreamCloser) handleWriteError(err error) {
	if s.closing.Load() {
		return
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		if s.log != nil && !errors.Is(netErr, &quicIdleTimeoutError) {
			s.log.Error().Err(netErr).Msg("closing QUIC stream due to timeout while writing")
		}
		s.stream.CancelWrite(0)
	}
}

func (s *runtimeSafeStreamCloser) Close() error {
	s.closing.Store(true)
	_ = s.stream.SetWriteDeadline(time.Now())

	s.lock.Lock()
	defer s.lock.Unlock()

	s.stream.CancelRead(0)
	return s.stream.Close()
}

func (s *runtimeSafeStreamCloser) CloseWrite() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.stream.Close()
}

func (s *runtimeSafeStreamCloser) SetDeadline(deadline time.Time) error {
	return s.stream.SetDeadline(deadline)
}
