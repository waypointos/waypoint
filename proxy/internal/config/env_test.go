package config

import (
	"testing"
)

func TestLoad_RequiresAll(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8081" {
		t.Fatal()
	}
	if cfg.WorkOSAPIKey != "sk_x" {
		t.Fatal()
	}
}

func TestLoad_FailsOnMissing(t *testing.T) {
	t.Setenv("PORT", "8081")
	for _, v := range []string{
		"DATABASE_URL", "OPERATOR_NKEY_SEED", "OPERATOR_PUBLIC_KEY",
		"WORKOS_API_KEY", "WORKOS_CLIENT_ID", "WORKOS_REDIRECT_URI",
		"WORKOS_COOKIE_PASSWORD", "PUBLIC_ORIGIN",
	} {
		t.Setenv(v, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoad_CookieSecureDefaultsTrue(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WAYPOINT_DEV", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CookieSecure {
		t.Fatal("CookieSecure should default to true in production")
	}
}

func TestLoad_CookieSecureOffWhenDev(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WAYPOINT_DEV", "1")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure should be false when WAYPOINT_DEV=1")
	}
}

// setRequiredEnv populates the mandatory env vars used by Load().
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PORT", "8081")
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("OPERATOR_NKEY_SEED", "c2VlZA==")
	t.Setenv("OPERATOR_PUBLIC_KEY", "OABCD")
	t.Setenv("WORKOS_API_KEY", "sk_x")
	t.Setenv("WORKOS_CLIENT_ID", "client_x")
	t.Setenv("WORKOS_REDIRECT_URI", "http://x/cb")
	t.Setenv("WORKOS_COOKIE_PASSWORD", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	t.Setenv("PUBLIC_ORIGIN", "http://x")
}
