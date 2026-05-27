package runtime

import cfdconnection "github.com/cloudflare/cloudflared/connection"

func toLegacyTunnelProperties(props *RuntimeTunnelProperties) *cfdconnection.TunnelProperties {
	if props == nil {
		return nil
	}
	return &cfdconnection.TunnelProperties{
		Credentials: cfdconnection.Credentials{
			AccountTag:   props.Credentials.AccountTag,
			TunnelSecret: append([]byte(nil), props.Credentials.TunnelSecret...),
			TunnelID:     props.Credentials.TunnelID,
			Endpoint:     props.Credentials.Endpoint,
		},
		QuickTunnelUrl: props.QuickTunnelURL,
	}
}
