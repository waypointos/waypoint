package landing

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndex_RendersLoginLink(t *testing.T) {
	h := New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.Index(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/auth/login"`) {
		t.Error("missing login anchor")
	}
	if !strings.Contains(body, "WAYPOINT") {
		t.Error("missing wordmark text")
	}
	if !strings.Contains(body, "<!doctype html>") {
		t.Error("missing doctype")
	}
}

func TestIndex_ContentType(t *testing.T) {
	h := New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.Index(rec, req)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestStatic_ServesAssets(t *testing.T) {
	h := New()
	req := httptest.NewRequest("GET", "/landing/static/landing.css", nil)
	rec := httptest.NewRecorder()
	http.StripPrefix("/landing/static/", h.Static()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}
}
