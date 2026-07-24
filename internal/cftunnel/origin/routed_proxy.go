package origin

import (
	"fmt"
	"net/http"
	"strings"

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
		passHostHeader := defaultTarget.PassHostHeader
		if rr.PassHostHeaderSet {
			passHostHeader = rr.PassHostHeader
		}
		target, err := ParseTarget(rr.Target, rr.OriginServerName, rr.InsecureSkipVerify, passHostHeader)
		if err != nil {
			return nil, fmt.Errorf("parse route[%d] target: %w", i, err)
		}
		key, err := routeRuleProxyKey(rr, passHostHeader)
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
	proxy, route, matched := p.pickRouteProxy(req.Host, req.URL.Path)
	if matched {
		req = requestWithStrippedPathPrefix(req, route)
	}
	proxy.ServeHTTP(w, req)
}

func (p *RoutedProxy) ProxyWebsocket(w protocol.ResponseWriter, req *http.Request) error {
	proxy, route, matched := p.pickRouteProxy(req.Host, req.URL.Path)
	if matched {
		req = requestWithStrippedPathPrefix(req, route)
	}
	return proxy.ProxyWebsocket(w, req)
}

func (p *RoutedProxy) pickRouteProxy(host, path string) (*Proxy, Route, bool) {
	if p.router == nil {
		return p.defaultProxy, Route{}, false
	}
	route, ok := p.router.Match(host, path)
	if !ok {
		return p.defaultProxy, Route{}, false
	}
	defaultPassHostHeader := false
	if p.defaultProxy != nil {
		defaultPassHostHeader = p.defaultProxy.target.PassHostHeader
	}
	if proxy, ok := p.proxyByTarget[matchedRouteProxyKey(route, defaultPassHostHeader)]; ok {
		return proxy, route, true
	}
	return p.defaultProxy, Route{}, false
}

func requestWithStrippedPathPrefix(req *http.Request, route Route) *http.Request {
	if !route.StripPathPrefix || route.Path == "" || route.Path == "/" || req.URL == nil {
		return req
	}
	if req.URL.Path != route.Path && !strings.HasPrefix(req.URL.Path, route.Path+"/") {
		return req
	}

	path := strings.TrimPrefix(req.URL.Path, route.Path)
	if path == "" {
		path = "/"
	}

	out := req.Clone(req.Context())
	u := *req.URL
	u.Path = path
	u.RawPath = ""
	out.URL = &u
	return out
}

func routeRuleProxyKey(route appconfig.RouteRule, passHostHeader bool) (string, error) {
	_, normalized, err := classifyAndNormalizeRulePath(route.Path)
	if err != nil {
		return "", err
	}
	return routeProxyKey(route.Host, normalized, route.Target, route.OriginServerName, route.InsecureSkipVerify, passHostHeader), nil
}

func matchedRouteProxyKey(route Route, defaultPassHostHeader bool) string {
	passHostHeader := defaultPassHostHeader
	if route.PassHostHeaderSet {
		passHostHeader = route.PassHostHeader
	}
	return routeProxyKey(route.Host, route.Path, route.Target, route.OriginServerName, route.InsecureSkipVerify, passHostHeader)
}

func routeProxyKey(host, path, target, serverName string, insecureSkipVerify, passHostHeader bool) string {
	return fmt.Sprintf(
		"host=%s,path=%s,target=%s,server_name=%s,insecure_skip_verify=%t,pass_host_header=%t",
		host,
		path,
		target,
		serverName,
		insecureSkipVerify,
		passHostHeader,
	)
}
