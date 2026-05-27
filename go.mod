module github.com/deanxv/cf-quicktunnel-ipv6pool

go 1.26.3

require (
	github.com/cloudflare/cloudflared v0.0.0
	github.com/rs/zerolog v1.20.0
	golang.org/x/sync v0.20.0
)

require (
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
)

require (
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/pprof v0.0.0-20250418163039-24c5476c6587 // indirect
	github.com/google/uuid v1.6.0
	github.com/onsi/ginkgo/v2 v2.23.4 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/quic-go/quic-go v0.52.0
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/mock v0.5.1 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/net v0.55.0
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
	zombiezen.com/go/capnproto2 v2.18.0+incompatible
)

replace github.com/cloudflare/cloudflared => ./third_party/cloudflared
