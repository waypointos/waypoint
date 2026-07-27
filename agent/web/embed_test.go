package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbed_servesIndex(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(strings.ToLower(string(b)), "<!doctype html") {
		t.Fatalf("expected dashboard index.html, got: %q", b)
	}
}

func TestEmbed_deepLinkFallsBackToIndex(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	// A client-router path with no backing file (the bug: this used to 404 on
	// a hard reload). It must return index.html so the SPA can resolve it.
	resp, err := http.Get(srv.URL + "/rover/sojourner/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(strings.ToLower(string(b)), "<!doctype html") {
		t.Fatalf("expected dashboard index.html for deep link, got: %q", b)
	}
}
