package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	EdgeProtocolQUIC  = "quic"
	EdgeProtocolHTTP2 = "http2"
)

type AppConfig struct {
	LogLevel        string
	LogFormat       string
	HealthListen    string
	ShutdownTimeout time.Duration
	CFTunnel        CFTunnelConfig
	Tunnels         []NamedTunnelConfig
}

type CFTunnelConfig struct {
	QuickService       string      `yaml:"quick_service"`
	TunnelToken        string      `yaml:"tunnel_token"`
	EdgeProtocol       string      `yaml:"edge_protocol"`
	Target             string      `yaml:"target"`
	OriginServerName   string      `yaml:"origin_server_name"`
	InsecureSkipVerify bool        `yaml:"insecure_skip_verify"`
	Routes             []RouteRule `yaml:"routes"`
}

type NamedTunnelConfig struct {
	Name     string         `yaml:"name"`
	CFTunnel CFTunnelConfig `yaml:"cf_tunnel"`
}

type RouteRule struct {
	Host                  string
	Path                  string
	Target                string
	OriginServerName      string
	InsecureSkipVerify    bool
	InsecureSkipVerifySet bool
}

func (r *RouteRule) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("route rule must be a mapping")
	}
	allowed := map[string]struct{}{
		"host":                 {},
		"path":                 {},
		"target":               {},
		"origin_server_name":   {},
		"insecure_skip_verify": {},
	}
	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i].Value
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("field %s not found in type config.RouteRule", key)
		}
	}
	var raw struct {
		Host               string `yaml:"host"`
		Path               string `yaml:"path"`
		Target             string `yaml:"target"`
		OriginServerName   string `yaml:"origin_server_name"`
		InsecureSkipVerify *bool  `yaml:"insecure_skip_verify"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	r.Host = raw.Host
	r.Path = raw.Path
	r.Target = raw.Target
	r.OriginServerName = raw.OriginServerName
	if raw.InsecureSkipVerify != nil {
		r.InsecureSkipVerify = *raw.InsecureSkipVerify
		r.InsecureSkipVerifySet = true
	}
	return nil
}

type routeFlag []RouteRule

func (r *routeFlag) String() string {
	if r == nil || len(*r) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*r))
	for _, rule := range *r {
		spec := rule.Path + "=" + rule.Target
		if rule.Host != "" {
			spec += ",host=" + rule.Host
		}
		if rule.OriginServerName != "" {
			spec += ",server_name=" + rule.OriginServerName
		}
		if rule.InsecureSkipVerifySet {
			spec += fmt.Sprintf(",insecure_skip_verify=%t", rule.InsecureSkipVerify)
		}
		parts = append(parts, spec)
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
	targetAndOptions := strings.TrimSpace(v[idx+1:])
	route, err := parseRouteTargetOptions(targetAndOptions)
	if err != nil {
		return err
	}
	route.Path = path
	target := route.Target
	if path == "" || target == "" {
		return errors.New("route rule must include non-empty path and target")
	}
	*r = append(*r, route)
	return nil
}

func parseRouteTargetOptions(v string) (RouteRule, error) {
	fields := strings.Split(v, ",")
	target := strings.TrimSpace(fields[0])
	if target == "" {
		return RouteRule{}, errors.New("route rule must include non-empty path and target")
	}

	route := RouteRule{Target: target}
	seen := map[string]struct{}{}
	for _, rawField := range fields[1:] {
		field := strings.TrimSpace(rawField)
		if field == "" {
			return RouteRule{}, errors.New("route rule has empty option")
		}
		idx := strings.Index(field, "=")
		if idx <= 0 || idx >= len(field)-1 {
			return RouteRule{}, fmt.Errorf("route option must be key=value: %q", field)
		}
		key := strings.ToLower(strings.TrimSpace(field[:idx]))
		value := strings.TrimSpace(field[idx+1:])
		if _, ok := seen[key]; ok {
			return RouteRule{}, fmt.Errorf("route option %q duplicated", key)
		}
		seen[key] = struct{}{}

		switch key {
		case "host":
			route.Host = strings.ToLower(strings.TrimSpace(value))
		case "server_name":
			route.OriginServerName = value
		case "insecure_skip_verify":
			switch value {
			case "true":
				route.InsecureSkipVerify = true
				route.InsecureSkipVerifySet = true
			case "false":
				route.InsecureSkipVerify = false
				route.InsecureSkipVerifySet = true
			default:
				return RouteRule{}, fmt.Errorf("route option insecure_skip_verify must be true or false: %s", value)
			}
		default:
			return RouteRule{}, fmt.Errorf("unsupported route option: %s", key)
		}
	}
	return route, nil
}

func Parse(args []string) (AppConfig, error) {
	cfg := AppConfig{}
	var configPath string

	fs := flag.NewFlagSet("cf-quicktunnel-ipv6pool", flag.ContinueOnError)
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "log level")
	fs.StringVar(&cfg.LogFormat, "log-format", "text", "log format")
	fs.StringVar(&cfg.HealthListen, "health-listen", ":9090", "health endpoint listen address")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", 10*time.Second, "maximum time to wait for runners to stop after shutdown starts")
	fs.StringVar(&configPath, "config", "", "optional YAML config file path")

	cfg.CFTunnel.QuickService = "https://api.trycloudflare.com"
	cfg.CFTunnel.TunnelToken = strings.TrimSpace(os.Getenv("CF_TUNNEL_TOKEN"))
	fs.StringVar(&cfg.CFTunnel.TunnelToken, "cf-tunnel-token", cfg.CFTunnel.TunnelToken, "Cloudflare remote-managed tunnel token")
	fs.StringVar(&cfg.CFTunnel.EdgeProtocol, "cf-edge-protocol", EdgeProtocolQUIC, "Cloudflare edge protocol")
	fs.StringVar(&cfg.CFTunnel.Target, "cf-tunnel-target", "", "origin target")
	fs.StringVar(&cfg.CFTunnel.OriginServerName, "cf-origin-server-name", "", "origin TLS server name override")
	fs.BoolVar(&cfg.CFTunnel.InsecureSkipVerify, "cf-origin-insecure-skip-verify", false, "skip origin TLS verification")
	fs.Var((*routeFlag)(&cfg.CFTunnel.Routes), "cf-route", "path-based route rule, repeatable, format: /path=url[,host=<hostname>][,server_name=<name>][,insecure_skip_verify=true|false]")

	if err := fs.Parse(args); err != nil {
		return AppConfig{}, err
	}
	if fs.NArg() != 0 {
		return AppConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if configPath != "" {
		if err := applyConfigFile(&cfg, configPath); err != nil {
			return AppConfig{}, err
		}
	}
	cfg.CFTunnel.TunnelToken = strings.TrimSpace(cfg.CFTunnel.TunnelToken)
	for i := range cfg.Tunnels {
		cfg.Tunnels[i].CFTunnel.TunnelToken = strings.TrimSpace(cfg.Tunnels[i].CFTunnel.TunnelToken)
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
	if len(c.Tunnels) > 0 {
		return validateNamedTunnels(c.Tunnels)
	}
	if err := c.CFTunnel.Validate(); err != nil {
		return err
	}
	return nil
}

func validateNamedTunnels(tunnels []NamedTunnelConfig) error {
	seen := make(map[string]struct{}, len(tunnels))
	for i, tunnel := range tunnels {
		name := strings.TrimSpace(tunnel.Name)
		if name == "" {
			return fmt.Errorf("tunnels[%d].name is required", i)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate tunnel name: %s", name)
		}
		seen[name] = struct{}{}
		if err := tunnel.CFTunnel.Validate(); err != nil {
			return fmt.Errorf("tunnels[%d] (%s): %w", i, name, err)
		}
	}
	return nil
}

type fileConfig struct {
	LogLevel        *string             `yaml:"log_level"`
	LogFormat       *string             `yaml:"log_format"`
	HealthListen    *string             `yaml:"health_listen"`
	ShutdownTimeout *string             `yaml:"shutdown_timeout"`
	CFTunnel        *CFTunnelConfig     `yaml:"cf_tunnel"`
	Tunnels         []NamedTunnelConfig `yaml:"tunnels"`
}

func applyConfigFile(cfg *AppConfig, path string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
	default:
		return fmt.Errorf("unsupported config file format %q: use YAML .yaml or .yml", filepath.Ext(path))
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	var fc fileConfig
	decoder := yaml.NewDecoder(strings.NewReader(string(b)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&fc); err != nil {
		return fmt.Errorf("decode YAML config file: %w", err)
	}
	if fc.LogLevel != nil {
		cfg.LogLevel = *fc.LogLevel
	}
	if fc.LogFormat != nil {
		cfg.LogFormat = *fc.LogFormat
	}
	if fc.HealthListen != nil {
		cfg.HealthListen = *fc.HealthListen
	}
	if fc.ShutdownTimeout != nil {
		v, err := time.ParseDuration(*fc.ShutdownTimeout)
		if err != nil {
			return fmt.Errorf("parse shutdown_timeout: %w", err)
		}
		cfg.ShutdownTimeout = v
	}
	if fc.CFTunnel != nil {
		if fc.CFTunnel.QuickService == "" {
			fc.CFTunnel.QuickService = cfg.CFTunnel.QuickService
		}
		cfg.CFTunnel = *fc.CFTunnel
	}
	if len(fc.Tunnels) > 0 {
		for i := range fc.Tunnels {
			if fc.Tunnels[i].CFTunnel.QuickService == "" {
				fc.Tunnels[i].CFTunnel.QuickService = cfg.CFTunnel.QuickService
			}
		}
		cfg.Tunnels = append([]NamedTunnelConfig(nil), fc.Tunnels...)
	}
	return nil
}

func (c CFTunnelConfig) Validate() error {
	if err := validateEdgeProtocol(c.EdgeProtocol); err != nil {
		return err
	}
	if c.Target == "" {
		return errors.New("cf-tunnel-target is required")
	}

	if err := validateTarget(c.Target); err != nil {
		return err
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
		host := strings.ToLower(strings.TrimSpace(route.Host))
		if host != "" {
			if err := validateRouteHost(host); err != nil {
				return fmt.Errorf("route[%d].host: %w", i, err)
			}
		}
		path := strings.TrimSpace(route.Path)
		if path == "" {
			return fmt.Errorf("route[%d].path is required", i)
		}
		target := strings.TrimSpace(route.Target)
		if target == "" {
			return fmt.Errorf("route[%d].target is required", i)
		}
		if err := validateTarget(target); err != nil {
			return fmt.Errorf("route[%d].target: %w", i, err)
		}
		kind, normalized, err := validateAndNormalizeRoutePath(path)
		if err != nil {
			return fmt.Errorf("route[%d].path: %w", i, err)
		}
		duplicateKey := routeDuplicateKey(host, normalized)
		switch kind {
		case "exact":
			if _, ok := exactSeen[duplicateKey]; ok {
				return fmt.Errorf("duplicate exact route path: %s", normalized)
			}
			exactSeen[duplicateKey] = struct{}{}
		case "prefix":
			if _, ok := prefixSeen[duplicateKey]; ok {
				return fmt.Errorf("duplicate prefix route path: %s/*", normalized)
			}
			prefixSeen[duplicateKey] = struct{}{}
		}
	}
	return nil
}

func validateRouteHost(host string) error {
	if strings.Contains(host, "://") {
		return errors.New("must be a hostname, not a URL")
	}
	if strings.ContainsAny(host, " /\\") {
		return errors.New("must not contain spaces or path separators")
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return errors.New("must not start or end with '.'")
	}
	if strings.Contains(host, "..") {
		return errors.New("must not contain empty labels")
	}
	return nil
}

func routeDuplicateKey(host, path string) string {
	return host + "\x00" + path
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

func validateTarget(target string) error {
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return errors.New("target must be a full URL with scheme and host")
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
		return nil
	default:
		return fmt.Errorf("unsupported target scheme: %s", u.Scheme)
	}
}
