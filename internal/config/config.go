package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	ProtocolAuto  = "auto"
	ProtocolHTTP  = "http"
	ProtocolHTTPS = "https"
	ProtocolWS    = "ws"
	ProtocolWSS   = "wss"

	EdgeProtocolQUIC  = "quic"
	EdgeProtocolHTTP2 = "http2"

)

type AppConfig struct {
	LogLevel        string
	LogFormat       string
	HealthListen    string
	ShutdownTimeout time.Duration
	CFTunnel        CFTunnelConfig
}

type CFTunnelConfig struct {
	QuickService       string
	EdgeProtocol       string
	HAConnections      int
	Target             string
	OriginProtocol     string
	OriginServerName   string
	InsecureSkipVerify bool
	Routes             []RouteRule
}

type RouteRule struct {
	Path   string
	Target string
}

type routeFlag []RouteRule

func (r *routeFlag) String() string {
	if r == nil || len(*r) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*r))
	for _, rule := range *r {
		parts = append(parts, rule.Path+"="+rule.Target)
	}
	return strings.Join(parts, ",")
}

func (r *routeFlag) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("route rule cannot be empty")
	}
	idx := strings.Index(v, "=")
	if idx <= 0 || idx >= len(v)-1 {
		return errors.New("route rule must be in format path=target")
	}
	path := strings.TrimSpace(v[:idx])
	target := strings.TrimSpace(v[idx+1:])
	if path == "" || target == "" {
		return errors.New("route rule must include non-empty path and target")
	}
	*r = append(*r, RouteRule{
		Path:   path,
		Target: target,
	})
	return nil
}

func Parse(args []string) (AppConfig, error) {
	cfg := AppConfig{}

	fs := flag.NewFlagSet("cf-quicktunnel-ipv6pool", flag.ContinueOnError)
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "log level")
	fs.StringVar(&cfg.LogFormat, "log-format", "text", "log format")
	fs.StringVar(&cfg.HealthListen, "health-listen", ":9090", "health endpoint listen address")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", 10*time.Second, "maximum time to wait for runners to stop after shutdown starts")

	fs.StringVar(&cfg.CFTunnel.QuickService, "cf-quick-service", "https://api.trycloudflare.com", "Quick Tunnel service base URL")
	fs.StringVar(&cfg.CFTunnel.EdgeProtocol, "cf-edge-protocol", EdgeProtocolQUIC, "Cloudflare edge protocol")
	fs.IntVar(&cfg.CFTunnel.HAConnections, "cf-ha-connections", 1, "Cloudflare Quick Tunnel edge connections; current Quick Tunnel runtime supports 1")
	fs.StringVar(&cfg.CFTunnel.Target, "cf-tunnel-target", "", "origin target")
	fs.StringVar(&cfg.CFTunnel.OriginProtocol, "cf-origin-protocol", ProtocolAuto, "origin protocol")
	fs.StringVar(&cfg.CFTunnel.OriginServerName, "cf-origin-server-name", "", "origin TLS server name override")
	fs.BoolVar(&cfg.CFTunnel.InsecureSkipVerify, "cf-origin-insecure-skip-verify", false, "skip origin TLS verification")
	fs.Var((*routeFlag)(&cfg.CFTunnel.Routes), "cf-route", "path-based route rule, repeatable, format: /path=host:port|url or /prefix/*=host:port|url")

	if err := fs.Parse(args); err != nil {
		return AppConfig{}, err
	}
	if fs.NArg() != 0 {
		return AppConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if err := cfg.Validate(); err != nil {
		return AppConfig{}, err
	}

	return cfg, nil
}

func (c AppConfig) Validate() error {
	if err := validateLogFormat(c.LogFormat); err != nil {
		return err
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown-timeout must be positive")
	}
	if err := c.CFTunnel.Validate(); err != nil {
		return err
	}
	return nil
}

func (c CFTunnelConfig) Validate() error {
	if err := validateEdgeProtocol(c.EdgeProtocol); err != nil {
		return err
	}
	if c.HAConnections != 1 {
		return errors.New("cf-ha-connections currently supports only 1 for Quick Tunnel")
	}
	if c.Target == "" {
		return errors.New("cf-tunnel-target is required")
	}

	targetHasScheme, err := validateTarget(c.Target)
	if err != nil {
		return err
	}
	if err := validateOriginProtocol(c.OriginProtocol); err != nil {
		return err
	}
	if !targetHasScheme && c.OriginProtocol == ProtocolAuto {
		return errors.New("cf-origin-protocol is required when cf-tunnel-target has no scheme")
	}
	if targetHasScheme && c.OriginProtocol != ProtocolAuto {
		if err := validateOriginProtocolOverride(c.Target, c.OriginProtocol); err != nil {
			return err
		}
	}
	if err := validateRouteRules(c.Routes); err != nil {
		return err
	}
	return nil
}

func validateRouteRules(routes []RouteRule) error {
	exactSeen := make(map[string]struct{})
	prefixSeen := make(map[string]struct{})
	for i, route := range routes {
		path := strings.TrimSpace(route.Path)
		if path == "" {
			return fmt.Errorf("route[%d].path is required", i)
		}
		if strings.TrimSpace(route.Target) == "" {
			return fmt.Errorf("route[%d].target is required", i)
		}
		kind, normalized, err := validateAndNormalizeRoutePath(path)
		if err != nil {
			return fmt.Errorf("route[%d].path: %w", i, err)
		}
		switch kind {
		case "exact":
			if _, ok := exactSeen[normalized]; ok {
				return fmt.Errorf("duplicate exact route path: %s", normalized)
			}
			exactSeen[normalized] = struct{}{}
		case "prefix":
			if _, ok := prefixSeen[normalized]; ok {
				return fmt.Errorf("duplicate prefix route path: %s/*", normalized)
			}
			prefixSeen[normalized] = struct{}{}
		}
	}
	return nil
}

func validateAndNormalizeRoutePath(path string) (kind string, normalized string, err error) {
	if path == "/" {
		return "exact", "/", nil
	}
	if !strings.HasPrefix(path, "/") {
		return "", "", errors.New("must start with '/'")
	}
	if strings.Contains(path, "//") {
		return "", "", errors.New("must not contain consecutive '/'")
	}

	if strings.HasSuffix(path, "/*") {
		base := strings.TrimSuffix(path, "/*")
		if base == "" || base == "/" {
			return "", "", errors.New("invalid prefix wildcard path")
		}
		if strings.Contains(base, "*") {
			return "", "", errors.New("wildcard '*' is only allowed as trailing '/*'")
		}
		return "prefix", base, nil
	}

	if strings.Contains(path, "*") {
		return "", "", errors.New("wildcard '*' is only allowed as trailing '/*'")
	}

	return "exact", path, nil
}

func validateLogFormat(v string) error {
	switch v {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("unsupported log-format: %s", v)
	}
}

func validateEdgeProtocol(v string) error {
	switch v {
	case EdgeProtocolQUIC, EdgeProtocolHTTP2:
		return nil
	default:
		return fmt.Errorf("unsupported cf-edge-protocol: %s", v)
	}
}

func validateOriginProtocol(v string) error {
	switch v {
	case ProtocolAuto, ProtocolHTTP, ProtocolHTTPS, ProtocolWS, ProtocolWSS:
		return nil
	default:
		return fmt.Errorf("unsupported cf-origin-protocol: %s", v)
	}
}

func validateTarget(target string) (bool, error) {
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil {
			return true, fmt.Errorf("invalid cf-tunnel-target URL: %w", err)
		}
		if u.Scheme == "" || u.Host == "" {
			return true, errors.New("cf-tunnel-target URL must include scheme and host")
		}
		return true, nil
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || port == "" {
		return false, errors.New("cf-tunnel-target must be a full URL or host:port")
	}
	return false, nil
}

func validateOriginProtocolOverride(target, originProtocol string) error {
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("invalid cf-tunnel-target URL: %w", err)
	}

	switch u.Scheme {
	case ProtocolHTTP:
		if originProtocol != ProtocolHTTP && originProtocol != ProtocolWS {
			return fmt.Errorf("cf-origin-protocol=%s is incompatible with target scheme=%s", originProtocol, u.Scheme)
		}
	case ProtocolHTTPS:
		if originProtocol != ProtocolHTTPS && originProtocol != ProtocolWSS {
			return fmt.Errorf("cf-origin-protocol=%s is incompatible with target scheme=%s", originProtocol, u.Scheme)
		}
	case ProtocolWS:
		if originProtocol != ProtocolWS && originProtocol != ProtocolHTTP {
			return fmt.Errorf("cf-origin-protocol=%s is incompatible with target scheme=%s", originProtocol, u.Scheme)
		}
	case ProtocolWSS:
		if originProtocol != ProtocolWSS && originProtocol != ProtocolHTTPS {
			return fmt.Errorf("cf-origin-protocol=%s is incompatible with target scheme=%s", originProtocol, u.Scheme)
		}
	default:
		return fmt.Errorf("unsupported target scheme: %s", u.Scheme)
	}

	return nil
}
