package crypto

import (
	"bytes"
	"testing"
)

func TestAESGCM_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	a, err := NewAESGCM(key)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}

	plaintext := []byte("hello waypoint")
	sealed, err := a.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Equal(sealed, plaintext) {
		t.Fatal("Seal returned plaintext unchanged")
	}

	got, err := a.Open(sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Open: got %q, want %q", got, plaintext)
	}
}

func TestAESGCM_SealProducesUniqueCiphertexts(t *testing.T) {
	key := make([]byte, 32)
	a, err := NewAESGCM(key)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}

	plaintext := []byte("same message")
	c1, err := a.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal 1: %v", err)
	}
	c2, err := a.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal 2: %v", err)
	}
	if bytes.Equal(c1, c2) {
		t.Fatal("two Seal calls produced identical output (nonce not random)")
	}
}

func TestAESGCM_WrongKeySize(t *testing.T) {
	_, err := NewAESGCM(make([]byte, 16))
	if err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}

func TestAESGCM_TamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	a, err := NewAESGCM(key)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}

	sealed, err := a.Seal([]byte("secret"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	sealed[len(sealed)-1] ^= 0xff // flip last byte
	_, err = a.Open(sealed)
	if err == nil {
		t.Fatal("Open accepted tampered ciphertext")
	}
}

func TestAESGCM_TooShort(t *testing.T) {
	key := make([]byte, 32)
	a, err := NewAESGCM(key)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	_, err = a.Open([]byte("short"))
	if err == nil {
		t.Fatal("Open accepted too-short input")
	}
}
