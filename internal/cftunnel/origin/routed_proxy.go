package origin

import (
	"fmt"
	"net/http"

	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/protocol"
)

type RoutedProxy struct {
	router        *Router
	defaultProxy  *Proxy
	proxyByTarget map[string]*Proxy
}

func NewRoutedProxy(defaultTarget Target, routes []appconfig.RouteRule) (*RoutedProxy, error) {
	defaultProxy := NewProxy(defaultTarget)
	if len(routes) == 0 {
		return &RoutedProxy{defaultProxy: defaultProxy}, nil
	}

	router, err := NewRouter(routes)
	if err != nil {
		return nil, err
	}

	proxyByTarget := make(map[string]*Proxy, len(routes)+1)
	proxyByTarget[""] = defaultProxy
	for i, rr := range routes {
		target, err := ParseTarget(rr.Target, string(defaultTarget.Protocol), defaultTarget.ServerName, defaultTarget.InsecureSkipVerify)
		if err != nil {
			return nil, fmt.Errorf("parse route[%d] target: %w", i, err)
		}
		key := rr.Target
		if _, ok := proxyByTarget[key]; !ok {
			proxyByTarget[key] = NewProxy(target)
		}
	}

	return &RoutedProxy{
		router:        router,
		defaultProxy:  defaultProxy,
		proxyByTarget: proxyByTarget,
	}, nil
}

func (p *RoutedProxy) Handler() http.Handler {
	return http.HandlerFunc(p.ServeHTTP)
}

func (p *RoutedProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	p.pickProxy(req.URL.Path).ServeHTTP(w, req)
}

func (p *RoutedProxy) ProxyWebsocket(w protocol.ResponseWriter, req *http.Request) error {
	return p.pickProxy(req.URL.Path).ProxyWebsocket(w, req)
}

func (p *RoutedProxy) pickProxy(path string) *Proxy {
	if p.router == nil {
		return p.defaultProxy
	}
	route, ok := p.router.Match(path)
	if !ok {
		return p.defaultProxy
	}
	if proxy, ok := p.proxyByTarget[route.Target]; ok {
		return proxy
	}
	return p.defaultProxy
}
