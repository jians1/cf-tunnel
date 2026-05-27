package socksproxy

import (
	"context"
	"io"
	"net"

	"github.com/rs/zerolog"

	"github.com/cloudflare/cloudflared/ingress"
	"github.com/cloudflare/cloudflared/ipaccess"
	"github.com/cloudflare/cloudflared/socks"
	"github.com/cloudflare/cloudflared/websocket"
)

type accessPolicyAdapter struct {
	policy *ipaccess.Policy
}

func (a accessPolicyAdapter) Allowed(ip net.IP, port int) (bool, string) {
	if a.policy == nil {
		return true, ""
	}
	allowed, rule := a.policy.Allowed(ip, port)
	if rule == nil {
		return allowed, ""
	}
	return allowed, rule.String()
}

type connection struct {
	accessPolicy *ipaccess.Policy
}

func (c *connection) Stream(ctx context.Context, tunnelConn io.ReadWriter, log *zerolog.Logger) {
	wsCtx, cancel := context.WithCancel(ctx)
	wsConn := websocket.NewConn(wsCtx, tunnelConn, log)
	socks.StreamNetHandler(wsConn, accessPolicyAdapter{policy: c.accessPolicy}, log)
	cancel()
	wsConn.Close()
}

func (c *connection) Close() error {
	return nil
}

func init() {
	ingress.RegisterStreamHandler("socks", func() ingress.StreamHandler {
		return socks.StreamHandler
	})
	ingress.RegisterManagedStreamOriginService(ingress.ServiceSocksProxy, ingress.ManagedStreamOriginServiceHooks{
		Start: func(_ *zerolog.Logger, _ <-chan struct{}, cfg ingress.OriginRequestConfig) (interface{}, error) {
			if len(cfg.IPRules) == 0 {
				return (*ipaccess.Policy)(nil), nil
			}
			rules := make([]ipaccess.Rule, len(cfg.IPRules))
			for i, ipRule := range cfg.IPRules {
				rule, err := ipaccess.NewRuleByCIDR(ipRule.Prefix, ipRule.Ports, ipRule.Allow)
				if err != nil {
					return nil, err
				}
				rules[i] = rule
			}
			return ipaccess.NewPolicy(false, rules)
		},
		Establish: func(state interface{}, _ context.Context, _ string, _ *zerolog.Logger) (ingress.OriginConnection, error) {
			policy, _ := state.(*ipaccess.Policy)
			return &connection{accessPolicy: policy}, nil
		},
	})
}
