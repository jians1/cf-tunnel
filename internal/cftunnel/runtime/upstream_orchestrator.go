package runtime

import (
	"encoding/json"
	"fmt"
)

type UpstreamOrchestrator struct {
	originProxy OriginProxy
	configJSON  []byte
}

func NewUpstreamOrchestrator(originProxy OriginProxy, session Session) (*UpstreamOrchestrator, error) {
	if originProxy == nil {
		return nil, fmt.Errorf("nil upstream origin proxy")
	}

	configJSON, err := json.Marshal(map[string]any{
		"quick_tunnel": true,
		"origin": map[string]any{
			"url":                    session.Origin.URL,
			"protocol":               string(session.Origin.Protocol),
			"server_name":            session.Origin.ServerName,
			"insecure_skip_verify":   session.Origin.InsecureSkipVerify,
			"websocket_upgrade_mode": session.Origin.WebsocketUpgradeMode,
		},
		"edge": map[string]any{
			"protocol":       session.Edge.Protocol,
			"ha_connections": session.HAConnections,
		},
		"hostname": session.Hostname,
		"url":      session.PublicURL,
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
