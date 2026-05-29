package origin

import (
	"fmt"
	"sort"
	"strings"

	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

type Route struct {
	Path                  string
	Target                string
	OriginServerName      string
	InsecureSkipVerify    bool
	InsecureSkipVerifySet bool
}

type Router struct {
	exact    map[string]Route
	prefixes []Route
	def      *Route
}

func NewRouter(rules []appconfig.RouteRule) (*Router, error) {
	r := &Router{
		exact: make(map[string]Route),
	}
	for i, rr := range rules {
		kind, normalized, err := classifyAndNormalizeRulePath(rr.Path)
		if err != nil {
			return nil, fmt.Errorf("build router: rule[%d] path %q: %w", i, rr.Path, err)
		}
		route := Route{
			Path:                  normalized,
			Target:                rr.Target,
			OriginServerName:      rr.OriginServerName,
			InsecureSkipVerify:    rr.InsecureSkipVerify,
			InsecureSkipVerifySet: rr.InsecureSkipVerifySet,
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
	return r, nil
}

func (r *Router) Match(path string) (Route, bool) {
	if r == nil {
		return Route{}, false
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
