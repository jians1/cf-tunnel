package origin

import (
	"fmt"
	"net/http"

	"github.com/jians1/cf-tunnel/internal/cftunnel/protocol"
	appconfig "github.com/jians1/cf-tunnel/internal/config"
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
		target, err := ParseTarget(rr.Target, rr.OriginServerName, rr.InsecureSkipVerify)
		if err != nil {
			return nil, fmt.Errorf("parse route[%d] target: %w", i, err)
		}
		key, err := routeRuleProxyKey(rr)
		if err != nil {
			return nil, fmt.Errorf("build route[%d] proxy key: %w", i, err)
		}
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
	p.pickProxy(req.Host, req.URL.Path).ServeHTTP(w, req)
}

func (p *RoutedProxy) ProxyWebsocket(w protocol.ResponseWriter, req *http.Request) error {
	return p.pickProxy(req.Host, req.URL.Path).ProxyWebsocket(w, req)
}

func (p *RoutedProxy) pickProxy(host, path string) *Proxy {
	if p.router == nil {
		return p.defaultProxy
	}
	route, ok := p.router.Match(host, path)
	if !ok {
		return p.defaultProxy
	}
	if proxy, ok := p.proxyByTarget[matchedRouteProxyKey(route)]; ok {
		return proxy
	}
	return p.defaultProxy
}

func routeRuleProxyKey(route appconfig.RouteRule) (string, error) {
	_, normalized, err := classifyAndNormalizeRulePath(route.Path)
	if err != nil {
		return "", err
	}
	return routeProxyKey(route.Host, normalized, route.Target, route.OriginServerName, route.InsecureSkipVerify), nil
}

func matchedRouteProxyKey(route Route) string {
	return routeProxyKey(route.Host, route.Path, route.Target, route.OriginServerName, route.InsecureSkipVerify)
}

func routeProxyKey(host, path, target, serverName string, insecureSkipVerify bool) string {
	return fmt.Sprintf(
		"host=%s,path=%s,target=%s,server_name=%s,insecure_skip_verify=%t",
		host,
		path,
		target,
		serverName,
		insecureSkipVerify,
	)
}
