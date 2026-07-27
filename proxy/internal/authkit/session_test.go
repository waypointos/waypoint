package authkit

import (
	"net/http/httptest"
	"testing"
)

func TestSession_RoundTrip(t *testing.T) {
	codec := NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false)
	rec := httptest.NewRecorder()
	if err := codec.Set(rec, Session{UserID: "abc-123"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	s, err := codec.Get(req)
	if err != nil {
		t.Fatal(err)
	}
	if s.UserID != "abc-123" {
		t.Fatalf("got %q", s.UserID)
	}
}

func TestSession_GetReturnsErrWhenAbsent(t *testing.T) {
	codec := NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false)
	req := httptest.NewRequest("GET", "/", nil)
	if _, err := codec.Get(req); err == nil {
		t.Fatal("expected error on missing cookie")
	}
}

func TestSession_SecureFlagWhenEnabled(t *testing.T) {
	codec := NewSessionCodec([]byte("0123456789012345678901234567890123456789"), true)
	rec := httptest.NewRecorder()
	if err := codec.Set(rec, Session{UserID: "x"}); err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if !cookies[0].Secure {
		t.Fatal("expected Secure flag")
	}
}

func TestSession_NoSecureFlagWhenDisabled(t *testing.T) {
	codec := NewSessionCodec([]byte("0123456789012345678901234567890123456789"), false)
	rec := httptest.NewRecorder()
	if err := codec.Set(rec, Session{UserID: "x"}); err != nil {
		t.Fatal(err)
	}
	if rec.Result().Cookies()[0].Secure {
		t.Fatal("expected no Secure flag in dev mode")
	}
}
