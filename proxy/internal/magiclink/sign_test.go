package magiclink

import (
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

func TestSignAndVerify(t *testing.T) {
	op, _ := nkeys.CreateOperator()
	pub, _ := op.PublicKey()

	tok, err := Mint(op, Claims{
		Sub: "uid-1", Email: "a@e.com", RoverID: "sim-01", Role: "control",
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Verify(pub, tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.RoverID != "sim-01" || c.Role != "control" {
		t.Fatal()
	}
}

func TestVerify_Expired(t *testing.T) {
	op, _ := nkeys.CreateOperator()
	pub, _ := op.PublicKey()
	tok, _ := Mint(op, Claims{Sub: "u", RoverID: "r", Role: "monitor", ExpiresAt: time.Now().Add(-1 * time.Second).Unix()})
	if _, err := Verify(pub, tok); err == nil {
		t.Fatal("must reject expired")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	op, _ := nkeys.CreateOperator()
	op2, _ := nkeys.CreateOperator()
	pub2, _ := op2.PublicKey()
	tok, _ := Mint(op, Claims{Sub: "u", RoverID: "r", Role: "admin", ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if _, err := Verify(pub2, tok); err == nil {
		t.Fatal("must reject signature from wrong operator")
	}
}
