package runtime

import (
	"fmt"

	"github.com/jians1/cf-tunnel/internal/cftunnel/origin"
)

func (s Session) OriginTarget() (origin.Target, error) {
	if s.Origin.URL == "" {
		return origin.Target{}, fmt.Errorf("missing origin url")
	}

	return origin.ParseTarget(
		s.Origin.URL,
		s.Origin.ServerName,
		s.Origin.InsecureSkipVerify,
		s.Origin.PassHostHeader,
	)
}
