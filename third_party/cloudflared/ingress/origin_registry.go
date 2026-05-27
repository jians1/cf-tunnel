package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"

	"github.com/rs/zerolog"
)

type NamedOriginServiceFactory func() OriginService
type ManagedHTTPOriginServiceStartFunc func(log *zerolog.Logger, shutdownC <-chan struct{}, cfg OriginRequestConfig) (*url.URL, error)
type ManagedStreamOriginServiceHooks struct {
	Start     func(log *zerolog.Logger, shutdownC <-chan struct{}, cfg OriginRequestConfig) (interface{}, error)
	Establish func(state interface{}, ctx context.Context, dest string, log *zerolog.Logger) (OriginConnection, error)
}
type StreamHandler func(originConn io.ReadWriter, remoteConn net.Conn, log *zerolog.Logger)
type StreamHandlerFactory func() StreamHandler

var (
	namedOriginServicesMu sync.RWMutex
	namedOriginServices   = map[string]NamedOriginServiceFactory{}
	streamHandlersMu      sync.RWMutex
	streamHandlers        = map[string]StreamHandlerFactory{}
)

func RegisterNamedOriginService(name string, factory NamedOriginServiceFactory) {
	namedOriginServicesMu.Lock()
	defer namedOriginServicesMu.Unlock()
	namedOriginServices[name] = factory
}

func NewNamedOriginService(name string) (OriginService, error) {
	namedOriginServicesMu.RLock()
	factory := namedOriginServices[name]
	namedOriginServicesMu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("unknown named origin service %q", name)
	}
	return factory(), nil
}

func RegisterStreamHandler(name string, factory StreamHandlerFactory) {
	streamHandlersMu.Lock()
	defer streamHandlersMu.Unlock()
	streamHandlers[name] = factory
}

func NewStreamHandler(name string) (StreamHandler, error) {
	streamHandlersMu.RLock()
	factory := streamHandlers[name]
	streamHandlersMu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("unknown stream handler %q", name)
	}
	return factory(), nil
}

type managedHTTPOriginService struct {
	name                  string
	url                   *url.URL
	disableOriginServerName bool
	startFunc             ManagedHTTPOriginServiceStartFunc
}

func RegisterManagedHTTPOriginService(name string, disableOriginServerName bool, start ManagedHTTPOriginServiceStartFunc) {
	RegisterNamedOriginService(name, func() OriginService {
		return &managedHTTPOriginService{
			name:                  name,
			disableOriginServerName: disableOriginServerName,
			startFunc:             start,
		}
	})
}

func (s *managedHTTPOriginService) String() string {
	return s.name
}

func (s *managedHTTPOriginService) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *managedHTTPOriginService) start(log *zerolog.Logger, shutdownC <-chan struct{}, cfg OriginRequestConfig) error {
	if s.startFunc == nil {
		return fmt.Errorf("managed origin service %q missing start func", s.name)
	}
	startedURL, err := s.startFunc(log, shutdownC, cfg)
	if err != nil {
		return err
	}
	s.url = startedURL
	return nil
}

func (s *managedHTTPOriginService) matchOriginServerName() bool {
	return !s.disableOriginServerName
}

type managedStreamOriginService struct {
	name  string
	state interface{}
	hooks ManagedStreamOriginServiceHooks
}

func RegisterManagedStreamOriginService(name string, hooks ManagedStreamOriginServiceHooks) {
	RegisterNamedOriginService(name, func() OriginService {
		return &managedStreamOriginService{
			name:  name,
			hooks: hooks,
		}
	})
}

func (s *managedStreamOriginService) String() string {
	return s.name
}

func (s *managedStreamOriginService) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *managedStreamOriginService) start(log *zerolog.Logger, shutdownC <-chan struct{}, cfg OriginRequestConfig) error {
	if s.hooks.Start == nil {
		return nil
	}
	state, err := s.hooks.Start(log, shutdownC, cfg)
	if err != nil {
		return err
	}
	s.state = state
	return nil
}

func (s *managedStreamOriginService) EstablishConnection(ctx context.Context, dest string, log *zerolog.Logger) (OriginConnection, error) {
	if s.hooks.Establish == nil {
		return nil, fmt.Errorf("managed stream origin service %q missing establish func", s.name)
	}
	return s.hooks.Establish(s.state, ctx, dest, log)
}
