package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewInstanceHTTP2(t *testing.T) {
	t.Parallel()

	instance, err := NewInstance(testSession(t, "http2"), nil)
	if err != nil {
		t.Fatalf("new instance: %v", err)
	}
	if instance.Prepared == nil {
		t.Fatal("expected prepared runtime")
	}
	if instance.UpstreamBinding == nil {
		t.Fatal("expected upstream binding")
	}
	if instance.HTTP2Server == nil {
		t.Fatal("expected http2 server")
	}
}

func TestNewInstanceQUICRequiresDialConfig(t *testing.T) {
	t.Parallel()

	_, err := NewInstance(testSession(t, "quic"), nil)
	if err == nil {
		t.Fatal("expected quic instance without dial config to fail")
	}
	if !errors.Is(err, errMissingQUICDialConfig) {
		t.Fatalf("expected missing quic dial config error, got: %v", err)
	}
}

func TestNewInstanceRejectsUnresolvedAutoProtocol(t *testing.T) {
	t.Parallel()

	_, err := NewInstance(testSession(t, "auto"), nil)
	if err == nil {
		t.Fatal("expected unresolved auto protocol error")
	}
	if !strings.Contains(err.Error(), "resolve edge protocol before building runtime instance") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstanceRunReturnsOnContextCancel(t *testing.T) {
	t.Parallel()

	instance, err := NewInstance(testSession(t, "http2"), nil)
	if err != nil {
		t.Fatalf("new instance: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- instance.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "connection with edge closed") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for instance run to stop")
	}
}

func TestInstanceRunRejectsNilInstance(t *testing.T) {
	t.Parallel()

	var instance *Instance
	err := instance.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "nil runtime instance") {
		t.Fatalf("unexpected error: %v", err)
	}
}
