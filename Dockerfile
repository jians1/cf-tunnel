FROM golang:1.26.3 AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY third_party/cloudflared ./third_party/cloudflared
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -buildvcs=false -trimpath -ldflags="-s -w" \
  -o /out/cf-tunnel ./cmd/app

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
  rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/cf-tunnel /usr/local/bin/cf-tunnel

ENTRYPOINT ["/usr/local/bin/cf-tunnel"]
