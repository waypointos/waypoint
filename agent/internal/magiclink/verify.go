package magiclink

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nkeys"
)

type Claims struct {
	Sub       string `json:"sub"`
	Email     string `json:"email"`
	RoverID   string `json:"rover"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

func Verify(operatorPub string, token string) (*Claims, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed token")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode sig: %w", err)
	}
	kp, err := nkeys.FromPublicKey(operatorPub)
	if err != nil {
		return nil, err
	}
	if err := kp.Verify(body, sig); err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, err
	}
	if time.Now().Unix() > c.ExpiresAt {
		return nil, errors.New("expired")
	}
	return &c, nil
}
