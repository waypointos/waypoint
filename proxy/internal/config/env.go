package config

import (
	"encoding/base64"
	"fmt"
	"os"
)

type Env struct {
	Port                 string
	DatabaseURL          string
	OperatorNKeySeed     []byte
	OperatorPublicKey    string
	WorkOSAPIKey         string
	WorkOSClientID       string
	WorkOSRedirectURI    string
	WorkOSCookiePassword []byte
	PublicOrigin         string
	CookieSecure         bool
	BasemapSource        string
}

func Load() (*Env, error) {
	get := func(k string) string { return os.Getenv(k) }
	required := []string{
		"DATABASE_URL", "OPERATOR_NKEY_SEED", "OPERATOR_PUBLIC_KEY",
		"WORKOS_API_KEY", "WORKOS_CLIENT_ID", "WORKOS_REDIRECT_URI",
		"WORKOS_COOKIE_PASSWORD", "PUBLIC_ORIGIN",
	}
	for _, k := range required {
		if get(k) == "" {
			return nil, fmt.Errorf("env: %s is required", k)
		}
	}
	seed, err := base64.StdEncoding.DecodeString(get("OPERATOR_NKEY_SEED"))
	if err != nil {
		return nil, fmt.Errorf("env: OPERATOR_NKEY_SEED is not valid base64: %w", err)
	}
	cookiePw, err := base64.StdEncoding.DecodeString(get("WORKOS_COOKIE_PASSWORD"))
	if err != nil {
		return nil, fmt.Errorf("env: WORKOS_COOKIE_PASSWORD is not valid base64: %w", err)
	}
	if len(cookiePw) < 32 {
		return nil, fmt.Errorf("env: WORKOS_COOKIE_PASSWORD must decode to at least 32 bytes")
	}
	port := get("PORT")
	if port == "" {
		port = "8081"
	}
	// Tile source: an explicit URL wins; otherwise derive an s3:// URL from the
	// Railway-injected bucket name (endpoint/region/creds come from AWS_* env);
	// otherwise fall back to a local directory for dev.
	basemapSource := get("WAYPOINT_PROXY_BASEMAP_URL")
	if basemapSource == "" {
		if b := get("AWS_S3_BUCKET_NAME"); b != "" {
			basemapSource = "s3://" + b
		} else {
			basemapSource = "/var/lib/waypoint-proxy/basemap"
		}
	}
	return &Env{
		Port:                 port,
		DatabaseURL:          get("DATABASE_URL"),
		OperatorNKeySeed:     seed,
		OperatorPublicKey:    get("OPERATOR_PUBLIC_KEY"),
		WorkOSAPIKey:         get("WORKOS_API_KEY"),
		WorkOSClientID:       get("WORKOS_CLIENT_ID"),
		WorkOSRedirectURI:    get("WORKOS_REDIRECT_URI"),
		WorkOSCookiePassword: cookiePw,
		PublicOrigin:         get("PUBLIC_ORIGIN"),
		CookieSecure:         os.Getenv("WAYPOINT_DEV") != "1",
		BasemapSource:        basemapSource,
	}, nil
}
