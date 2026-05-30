package cftunnel

import (
	"context"

	"github.com/google/uuid"

	"github.com/jians1/cf-tunnel/internal/cftunnel/api"
	tunnelconfig "github.com/jians1/cf-tunnel/internal/cftunnel/config"
	"github.com/jians1/cf-tunnel/internal/cftunnel/credentials"
)

func mockQuickTunnelReservationFunc() quickTunnelReservationFunc {
	return func(ctx context.Context, runtimeConfig tunnelconfig.RuntimeConfig) (*api.QuickTunnelReservation, error) {
		_ = ctx
		_ = runtimeConfig
		return &api.QuickTunnelReservation{
			Credentials: credentials.Credentials{
				AccountTag:   "acct",
				TunnelSecret: []byte("secret"),
				TunnelID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			},
			Hostname: "demo.trycloudflare.com",
			URL:      "https://demo.trycloudflare.com",
			Name:     "demo",
		}, nil
	}
}
