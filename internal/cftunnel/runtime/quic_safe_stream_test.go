package runtime

import (
	"context"
	"testing"
	"time"

	quicgo "github.com/quic-go/quic-go"
)

func TestRuntimeSafeStreamCloserCancelsReadAndClosesWrite(t *testing.T) {
	t.Parallel()

	stream := &recordingQUICStream{ctx: context.Background()}
	logger := newTestZeroLogger()
	closer := newRuntimeSafeStreamCloser(stream, time.Second, &logger)

	if err := closer.Close(); err != nil {
		t.Fatalf("close safe stream: %v", err)
	}
	if !stream.readCanceled {
		t.Fatal("expected read side to be canceled")
	}
	if !stream.closed {
		t.Fatal("expected write side to be closed")
	}
	if stream.writeDeadline.IsZero() {
		t.Fatal("expected close to force a write deadline")
	}
}

func TestRuntimeSafeStreamCloserSetsWriteDeadline(t *testing.T) {
	t.Parallel()

	stream := &recordingQUICStream{ctx: context.Background()}
	logger := newTestZeroLogger()
	closer := newRuntimeSafeStreamCloser(stream, time.Second, &logger)

	n, err := closer.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write safe stream: %v", err)
	}
	if n != 5 {
		t.Fatalf("unexpected bytes written: got %d", n)
	}
	if stream.writeDeadline.IsZero() {
		t.Fatal("expected write deadline")
	}
}

type recordingQUICStream struct {
	ctx           context.Context
	written       []byte
	closed        bool
	readCanceled  bool
	writeCanceled bool
	readDeadline  time.Time
	writeDeadline time.Time
	deadline      time.Time
}

func (s *recordingQUICStream) StreamID() quicgo.StreamID { return 0 }

func (s *recordingQUICStream) Read([]byte) (int, error) { return 0, nil }

func (s *recordingQUICStream) Write(p []byte) (int, error) {
	s.written = append(s.written, p...)
	return len(p), nil
}

func (s *recordingQUICStream) Close() error {
	s.closed = true
	return nil
}

func (s *recordingQUICStream) CancelRead(quicgo.StreamErrorCode) {
	s.readCanceled = true
}

func (s *recordingQUICStream) CancelWrite(quicgo.StreamErrorCode) {
	s.writeCanceled = true
}

func (s *recordingQUICStream) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *recordingQUICStream) SetReadDeadline(t time.Time) error {
	s.readDeadline = t
	return nil
}

func (s *recordingQUICStream) SetWriteDeadline(t time.Time) error {
	s.writeDeadline = t
	return nil
}

func (s *recordingQUICStream) SetDeadline(t time.Time) error {
	s.deadline = t
	return nil
}
