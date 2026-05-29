package credentials

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type TunnelToken struct {
	AccountTag   string    `json:"a"`
	TunnelSecret []byte    `json:"s"`
	TunnelID     uuid.UUID `json:"t"`
	Endpoint     string    `json:"e,omitempty"`
}

func ParseTunnelToken(token string) (Credentials, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Credentials{}, errors.New("missing tunnel token")
	}
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return Credentials{}, fmt.Errorf("decode tunnel token: %w", err)
	}

	var parsed TunnelToken
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Credentials{}, fmt.Errorf("unmarshal tunnel token: %w", err)
	}
	if strings.TrimSpace(parsed.AccountTag) == "" {
		return Credentials{}, errors.New("missing tunnel token account tag")
	}
	if parsed.TunnelID == uuid.Nil {
		return Credentials{}, errors.New("missing tunnel token tunnel id")
	}
	if len(parsed.TunnelSecret) == 0 {
		return Credentials{}, errors.New("missing tunnel token secret")
	}

	return Credentials{
		AccountTag:   strings.TrimSpace(parsed.AccountTag),
		TunnelSecret: append([]byte(nil), parsed.TunnelSecret...),
		TunnelID:     parsed.TunnelID,
		Endpoint:     strings.TrimSpace(parsed.Endpoint),
	}, nil
}
