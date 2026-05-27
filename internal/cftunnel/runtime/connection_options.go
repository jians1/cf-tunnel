package runtime

import (
	goruntime "runtime"

	cfdclient "github.com/cloudflare/cloudflared/client"
	"github.com/cloudflare/cloudflared/features"
)

const runtimeClientVersion = "cf-quicktunnel-ipv6pool/0.1.0-prototype"

type runtimeFeatureSelector struct{}

func (runtimeFeatureSelector) Snapshot() features.FeatureSnapshot {
	return features.FeatureSnapshot{
		PostQuantum:     features.PostQuantumPrefer,
		DatagramVersion: features.DatagramV2,
		FeaturesList: []string{
			features.FeatureAllowRemoteConfig,
			features.FeatureSerializedHeaders,
			features.FeatureDatagramV2,
			features.FeatureQUICSupportEOF,
			features.FeatureManagementLogs,
		},
	}
}

func newRuntimeConnectionOptions() (*cfdclient.ConnectionOptionsSnapshot, error) {
	cfg, err := cfdclient.NewConfig(runtimeClientVersion, goruntime.GOOS+"_"+goruntime.GOARCH, runtimeFeatureSelector{})
	if err != nil {
		return nil, err
	}
	return cfg.ConnectionOptionsSnapshot(nil, 0), nil
}
