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

	IPv6StrategyRandom = "random"
)

type AppConfig struct {
	LogLevel        string
	LogFormat       string
	HealthListen    string
	ShutdownTimeout time.Duration
	CFTunnel        CFTunnelConfig
	IPv6Pool        IPv6PoolConfig
}

type CFTunnelConfig struct {
	Enabled                  bool
	QuickService             string
	QuickServiceTimeout      time.Duration
	QuickServiceRetryBackoff string
	EdgeProtocol             string
	HAConnections            int
	Target                   string
	OriginProtocol           string
	OriginServerName         string
	InsecureSkipVerify       bool
}

type IPv6PoolConfig struct {
	Enabled       bool
	HTTPListen    string
	SOCKS5Listen  string
	BindInterface string
	CIDR          string
	Strategy      string
}

func Parse(args []string) (AppConfig, error) {
	cfg := AppConfig{}

	fs := flag.NewFlagSet("cf-quicktunnel-ipv6pool", flag.ContinueOnError)
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "log level")
	fs.StringVar(&cfg.LogFormat, "log-format", "text", "log format")
	fs.StringVar(&cfg.HealthListen, "health-listen", ":9090", "health endpoint listen address")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", 10*time.Second, "maximum time to wait for runners to stop after shutdown starts")

	fs.BoolVar(&cfg.CFTunnel.Enabled, "enable-cf-tunnel", false, "enable Cloudflare quick tunnel")
	fs.StringVar(&cfg.CFTunnel.QuickService, "cf-quick-service", "https://api.trycloudflare.com", "Quick Tunnel service base URL")
	fs.DurationVar(&cfg.CFTunnel.QuickServiceTimeout, "cf-quick-service-timeout", 15*time.Second, "Quick Tunnel service request timeout")
	fs.StringVar(&cfg.CFTunnel.QuickServiceRetryBackoff, "cf-quick-service-retry-backoff", "500ms,1500ms", "comma-separated retry backoffs for Quick Tunnel service rate limits")
	fs.StringVar(&cfg.CFTunnel.EdgeProtocol, "cf-edge-protocol", EdgeProtocolQUIC, "Cloudflare edge protocol")
	fs.IntVar(&cfg.CFTunnel.HAConnections, "cf-ha-connections", 1, "Cloudflare Quick Tunnel edge connections; current Quick Tunnel runtime supports 1")
	fs.StringVar(&cfg.CFTunnel.Target, "cf-tunnel-target", "", "origin target")
	fs.StringVar(&cfg.CFTunnel.OriginProtocol, "cf-origin-protocol", ProtocolAuto, "origin protocol")
	fs.StringVar(&cfg.CFTunnel.OriginServerName, "cf-origin-server-name", "", "origin TLS server name override")
	fs.BoolVar(&cfg.CFTunnel.InsecureSkipVerify, "cf-origin-insecure-skip-verify", false, "skip origin TLS verification")

	fs.BoolVar(&cfg.IPv6Pool.Enabled, "enable-ipv6-pool", false, "enable IPv6 pool proxy")
	fs.StringVar(&cfg.IPv6Pool.HTTPListen, "ipv6-pool-http", ":3128", "IPv6 pool HTTP proxy listen address")
	fs.StringVar(&cfg.IPv6Pool.SOCKS5Listen, "ipv6-pool-socks5", ":3129", "IPv6 pool SOCKS5 proxy listen address")
	fs.StringVar(&cfg.IPv6Pool.BindInterface, "ipv6-pool-bind-interface", "", "network interface used to source IPv6 addresses")
	fs.StringVar(&cfg.IPv6Pool.CIDR, "ipv6-pool-cidr", "", "IPv6 source CIDR")
	fs.StringVar(&cfg.IPv6Pool.Strategy, "ipv6-pool-strategy", IPv6StrategyRandom, "IPv6 source selection strategy")

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
	if !c.CFTunnel.Enabled && !c.IPv6Pool.Enabled {
		return errors.New("at least one feature must be enabled")
	}
	if err := validateLogFormat(c.LogFormat); err != nil {
		return err
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown-timeout must be positive")
	}
	if err := c.CFTunnel.Validate(); err != nil {
		return err
	}
	if err := c.IPv6Pool.Validate(); err != nil {
		return err
	}
	return nil
}

func (c CFTunnelConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if err := validateEdgeProtocol(c.EdgeProtocol); err != nil {
		return err
	}
	if c.QuickServiceTimeout <= 0 {
		return errors.New("cf-quick-service-timeout must be positive")
	}
	if _, err := parseDurationList(c.QuickServiceRetryBackoff); err != nil {
		return fmt.Errorf("invalid cf-quick-service-retry-backoff: %w", err)
	}
	if c.HAConnections != 1 {
		return errors.New("cf-ha-connections currently supports only 1 for Quick Tunnel")
	}
	if c.Target == "" {
		return errors.New("cf-tunnel-target is required when enable-cf-tunnel=true")
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
	return nil
}

func (c IPv6PoolConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.HTTPListen == "" && c.SOCKS5Listen == "" {
		return errors.New("at least one IPv6 pool listener must be set")
	}
	if c.CIDR == "" && c.BindInterface == "" {
		return errors.New("ipv6-pool-cidr or ipv6-pool-bind-interface is required when enable-ipv6-pool=true")
	}
	if err := validateIPv6Strategy(c.Strategy); err != nil {
		return err
	}
	if c.CIDR != "" {
		if _, _, err := net.ParseCIDR(c.CIDR); err != nil {
			return fmt.Errorf("invalid ipv6-pool-cidr: %w", err)
		}
	}
	return nil
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

func validateIPv6Strategy(v string) error {
	switch v {
	case IPv6StrategyRandom:
		return nil
	default:
		return fmt.Errorf("unsupported ipv6-pool-strategy: %s", v)
	}
}

func parseDurationList(v string) ([]time.Duration, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	durations := make([]time.Duration, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, errors.New("empty duration")
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return nil, err
		}
		if duration < 0 {
			return nil, errors.New("duration must not be negative")
		}
		durations = append(durations, duration)
	}
	return durations, nil
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
