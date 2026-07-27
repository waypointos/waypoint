package magiclink

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

func mint(t *testing.T, signer nkeys.KeyPair, c Claims) string {
	t.Helper()
	nonceBuf := make([]byte, 12)
	_, _ = rand.Read(nonceBuf)
	c.Nonce = base64.RawURLEncoding.EncodeToString(nonceBuf)
	c.IssuedAt = time.Now().Unix()
	body, _ := json.Marshal(c)
	sig, err := signer.Sign(body)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestVerify_RoundTrip(t *testing.T) {
	op, _ := nkeys.CreateOperator()
	pub, _ := op.PublicKey()
	tok := mint(t, op, Claims{Sub: "u", RoverID: "sim-01", Role: "control", ExpiresAt: time.Now().Add(5 * time.Minute).Unix()})
	c, err := Verify(pub, tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.RoverID != "sim-01" {
		t.Fatal()
	}
}

func TestVerify_Expired(t *testing.T) {
	op, _ := nkeys.CreateOperator()
	pub, _ := op.PublicKey()
	tok := mint(t, op, Claims{Sub: "u", ExpiresAt: time.Now().Add(-1 * time.Second).Unix()})
	if _, err := Verify(pub, tok); err == nil {
		t.Fatal("must reject expired")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	op, _ := nkeys.CreateOperator()
	other, _ := nkeys.CreateOperator()
	otherPub, _ := other.PublicKey()
	tok := mint(t, op, Claims{Sub: "u", ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if _, err := Verify(otherPub, tok); err == nil {
		t.Fatal("must reject wrong-key signature")
	}
}
