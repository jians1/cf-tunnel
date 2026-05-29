package runtime

import (
	"encoding/json"
	"fmt"
)

type UpstreamOrchestrator struct {
	originProxy OriginProxy
	configJSON  []byte
}

type upstreamOrchestratorConfig struct {
	QuickTunnel bool                           `json:"quick_tunnel"`
	Origin      upstreamOrchestratorOrigin     `json:"origin"`
	Edge        upstreamOrchestratorEdge       `json:"edge"`
	Hostname    string                         `json:"hostname"`
	URL         string                         `json:"url"`
}

type upstreamOrchestratorOrigin struct {
	URL                  string `json:"url"`
	Protocol             string `json:"protocol"`
	ServerName           string `json:"server_name"`
	InsecureSkipVerify   bool   `json:"insecure_skip_verify"`
	WebsocketUpgradeMode bool   `json:"websocket_upgrade_mode"`
}

type upstreamOrchestratorEdge struct {
	Protocol      string `json:"protocol"`
	HAConnections int    `json:"ha_connections"`
}

func NewUpstreamOrchestrator(originProxy OriginProxy, session Session) (*UpstreamOrchestrator, error) {
	if originProxy == nil {
		return nil, fmt.Errorf("nil upstream origin proxy")
	}

	configJSON, err := json.Marshal(upstreamOrchestratorConfig{
		QuickTunnel: session.QuickTunnel,
		Origin: upstreamOrchestratorOrigin{
			URL:                  session.Origin.URL,
			Protocol:             string(session.Origin.Protocol),
			ServerName:           session.Origin.ServerName,
			InsecureSkipVerify:   session.Origin.InsecureSkipVerify,
			WebsocketUpgradeMode: session.Origin.WebsocketUpgradeMode,
		},
		Edge: upstreamOrchestratorEdge{
			Protocol:      session.Edge.Protocol,
			HAConnections: session.HAConnections,
		},
		Hostname: session.Hostname,
		URL:      session.PublicURL,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal orchestrator config: %w", err)
	}

	return &UpstreamOrchestrator{
		originProxy: originProxy,
		configJSON:  configJSON,
	}, nil
}

func (o *UpstreamOrchestrator) UpdateConfig(version int32, _ []byte) *runtimeUpdateConfigurationResponse {
	return &runtimeUpdateConfigurationResponse{
		LastAppliedVersion: version,
	}
}

func (o *UpstreamOrchestrator) GetConfigJSON() ([]byte, error) {
	return append([]byte(nil), o.configJSON...), nil
}

func (o *UpstreamOrchestrator) GetOriginProxy() (OriginProxy, error) {
	return o.originProxy, nil
}
