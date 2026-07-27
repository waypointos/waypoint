package modules

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOwnerRepo(t *testing.T) {
	o, r, err := ParseOwnerRepo("https://github.com/acme/wp-power")
	require.NoError(t, err)
	require.Equal(t, "acme", o)
	require.Equal(t, "wp-power", r)

	_, _, err = ParseOwnerRepo("https://example.com/x")
	require.Error(t, err)
}

func TestGitHub_ListReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/acme/wp-power/releases", r.URL.Path)
		require.Equal(t, "token gh_x", r.Header.Get("Authorization"))
		io.WriteString(w, `[
		  {"tag_name":"v1.5.0","body":"notes","html_url":"https://gh/r/1.5.0","published_at":"2026-05-20T00:00:00Z",
		   "assets":[{"name":"manifest.json","url":"A"},{"name":"power-monitor-1.5.0.raw","url":"B"}]}
		]`)
	}))
	defer srv.Close()

	gh := &GitHub{HTTP: srv.Client(), APIBase: srv.URL}
	rels, err := gh.ListReleases(context.Background(), "acme", "wp-power", "gh_x")
	require.NoError(t, err)
	require.Len(t, rels, 1)
	require.Equal(t, "v1.5.0", rels[0].TagName)
	require.Equal(t, "notes", rels[0].Body)
	require.Len(t, rels[0].Assets, 2)
}

func TestGitHub_Download(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/octet-stream", r.Header.Get("Accept"))
		io.WriteString(w, "blobbytes")
	}))
	defer srv.Close()

	gh := &GitHub{HTTP: srv.Client(), APIBase: srv.URL}
	b, err := gh.Download(context.Background(), srv.URL+"/asset", "")
	require.NoError(t, err)
	require.Equal(t, "blobbytes", string(b))
}

func TestGitHub_Readme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/acme/wp-power/readme", r.URL.Path)
		require.Equal(t, "application/vnd.github.raw", r.Header.Get("Accept"))
		io.WriteString(w, "# Power Monitor")
	}))
	defer srv.Close()

	gh := &GitHub{HTTP: srv.Client(), APIBase: srv.URL}
	b, err := gh.Readme(context.Background(), "acme", "wp-power", "")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(b), "# Power Monitor"))
}
