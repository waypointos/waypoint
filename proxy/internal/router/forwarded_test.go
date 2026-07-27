package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustForwarded_RewritesSchemeAndHost(t *testing.T) {
	var sawScheme, sawHost string
	h := TrustForwarded(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawScheme = r.URL.Scheme
		sawHost = r.Host
	}))

	req := httptest.NewRequest("GET", "http://internal:8081/x", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "proxy.example.com")

	h.ServeHTTP(httptest.NewRecorder(), req)

	if sawScheme != "https" {
		t.Errorf("scheme = %q, want https", sawScheme)
	}
	if sawHost != "proxy.example.com" {
		t.Errorf("host = %q, want proxy.example.com", sawHost)
	}
}

func TestTrustForwarded_LeavesUnsetHeadersAlone(t *testing.T) {
	var sawScheme, sawHost string
	h := TrustForwarded(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawScheme = r.URL.Scheme
		sawHost = r.Host
	}))
	req := httptest.NewRequest("GET", "http://internal:8081/x", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if sawScheme == "https" {
		t.Error("scheme should not have been set to https without header")
	}
	if sawHost != "internal:8081" {
		t.Errorf("host = %q, want internal:8081", sawHost)
	}
}
