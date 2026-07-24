package origin

import (
	"fmt"
	"net"
	"sort"
	"strings"

	appconfig "github.com/jians1/cf-tunnel/internal/config"
)

type Route struct {
	Host                  string
	Path                  string
	Target                string
	StripPathPrefix       bool
	OriginServerName      string
	InsecureSkipVerify    bool
	InsecureSkipVerifySet bool
	PassHostHeader        bool
	PassHostHeaderSet     bool
}

type Router struct {
	hostExact    map[string]Route
	hostPrefixes []Route
	hostDef      map[string]Route
	exact        map[string]Route
	prefixes     []Route
	def          *Route
}

func NewRouter(rules []appconfig.RouteRule) (*Router, error) {
	r := &Router{
		hostExact: make(map[string]Route),
		hostDef:   make(map[string]Route),
		exact:     make(map[string]Route),
	}
	for i, rr := range rules {
		kind, normalized, err := classifyAndNormalizeRulePath(rr.Path)
		if err != nil {
			return nil, fmt.Errorf("build router: rule[%d] path %q: %w", i, rr.Path, err)
		}
		route := Route{
			Host:                  normalizeRouteHost(rr.Host),
			Path:                  normalized,
			Target:                rr.Target,
			StripPathPrefix:       rr.StripPathPrefix,
			OriginServerName:      rr.OriginServerName,
			InsecureSkipVerify:    rr.InsecureSkipVerify,
			InsecureSkipVerifySet: rr.InsecureSkipVerifySet,
			PassHostHeader:        rr.PassHostHeader,
			PassHostHeaderSet:     rr.PassHostHeaderSet,
		}
		if route.Host != "" {
			if err := addHostRoute(r, kind, route); err != nil {
				return nil, err
			}
			continue
		}
		switch kind {
		case "default":
			if r.def != nil {
				return nil, fmt.Errorf("build router: duplicate default route")
			}
			cp := route
			r.def = &cp
		case "exact":
			if _, ok := r.exact[normalized]; ok {
				return nil, fmt.Errorf("build router: duplicate exact route %q", normalized)
			}
			r.exact[normalized] = route
		case "prefix":
			for _, p := range r.prefixes {
				if p.Path == normalized {
					return nil, fmt.Errorf("build router: duplicate prefix route %q", normalized)
				}
			}
			r.prefixes = append(r.prefixes, route)
		}
	}

	sort.Slice(r.prefixes, func(i, j int) bool {
		return len(r.prefixes[i].Path) > len(r.prefixes[j].Path)
	})
	sort.Slice(r.hostPrefixes, func(i, j int) bool {
		return len(r.hostPrefixes[i].Path) > len(r.hostPrefixes[j].Path)
	})
	return r, nil
}

func addHostRoute(r *Router, kind string, route Route) error {
	key := routeHostPathKey(route.Host, route.Path)
	switch kind {
	case "default":
		if _, ok := r.hostDef[route.Host]; ok {
			return fmt.Errorf("build router: duplicate default route for host %q", route.Host)
		}
		r.hostDef[route.Host] = route
	case "exact":
		if _, ok := r.hostExact[key]; ok {
			return fmt.Errorf("build router: duplicate exact route %q for host %q", route.Path, route.Host)
		}
		r.hostExact[key] = route
	case "prefix":
		for _, p := range r.hostPrefixes {
			if p.Host == route.Host && p.Path == route.Path {
				return fmt.Errorf("build router: duplicate prefix route %q for host %q", route.Path, route.Host)
			}
		}
		r.hostPrefixes = append(r.hostPrefixes, route)
	}
	return nil
}

func (r *Router) Match(host, path string) (Route, bool) {
	if r == nil {
		return Route{}, false
	}
	host = normalizeRequestHost(host)
	if host != "" {
		if route, ok := r.hostExact[routeHostPathKey(host, path)]; ok {
			return route, true
		}
		for _, p := range r.hostPrefixes {
			if p.Host == host && (path == p.Path || strings.HasPrefix(path, p.Path+"/")) {
				return p, true
			}
		}
		if route, ok := r.hostDef[host]; ok {
			return route, true
		}
	}
	if route, ok := r.exact[path]; ok {
		return route, true
	}
	for _, p := range r.prefixes {
		if path == p.Path || strings.HasPrefix(path, p.Path+"/") {
			return p, true
		}
	}
	if r.def != nil {
		return *r.def, true
	}
	return Route{}, false
}

func routeHostPathKey(host, path string) string {
	return host + "\x00" + path
}

func normalizeRouteHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func normalizeRequestHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return strings.Trim(strings.ToLower(h), "[]")
	}
	return strings.Trim(host, "[]")
}

func classifyAndNormalizeRulePath(path string) (kind string, normalized string, err error) {
	if strings.TrimSpace(path) != path {
		return "", "", fmt.Errorf("must not contain leading or trailing spaces")
	}
	if path == "/" {
		return "default", "/", nil
	}
	if !strings.HasPrefix(path, "/") {
		return "", "", fmt.Errorf("must start with '/'")
	}
	if strings.Contains(path, "//") {
		return "", "", fmt.Errorf("must not contain consecutive '/'")
	}
	if strings.HasSuffix(path, "/*") {
		base := strings.TrimSuffix(path, "/*")
		if base == "" || base == "/" {
			return "", "", fmt.Errorf("invalid prefix wildcard path")
		}
		if strings.Contains(base, "*") {
			return "", "", fmt.Errorf("wildcard '*' is only allowed as trailing '/*'")
		}
		return "prefix", base, nil
	}
	if strings.Contains(path, "*") {
		return "", "", fmt.Errorf("wildcard '*' is only allowed as trailing '/*'")
	}
	return "exact", path, nil
}
