package hello

import (
	"net/url"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/cloudflare/cloudflared/ingress"
)

func init() {
	ingress.RegisterManagedHTTPOriginService(ingress.HelloWorldService, true, startManagedHelloWorld)
}

func startManagedHelloWorld(log *zerolog.Logger, shutdownC <-chan struct{}, _ ingress.OriginRequestConfig) (*url.URL, error) {
	helloListener, err := CreateTLSListener("127.0.0.1:")
	if err != nil {
		return nil, errors.Wrap(err, "Cannot start Hello World Server")
	}
	go StartHelloWorldServer(log, helloListener, shutdownC)
	return &url.URL{
		Scheme: "https",
		Host:   helloListener.Addr().String(),
	}, nil
}
