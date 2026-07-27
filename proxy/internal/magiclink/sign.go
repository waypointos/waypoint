package magiclink

import (
	"crypto/rand"
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

func Mint(signer nkeys.KeyPair, c Claims) (string, error) {
	nonceBuf := make([]byte, 12)
	_, _ = rand.Read(nonceBuf)
	c.Nonce = base64.RawURLEncoding.EncodeToString(nonceBuf)
	c.IssuedAt = time.Now().Unix()

	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sig, err := signer.Sign(body)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
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
		return nil, fmt.Errorf("operator pubkey: %w", err)
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
