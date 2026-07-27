package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStaticToken_RoundTrip(t *testing.T) {
	key := DeriveStaticKey([]byte("operator-seed"))
	now := time.Unix(1_700_000_000, 0)
	tok := MintStaticToken(key, "cosmos", "so100", now.Add(5*time.Minute))

	require.True(t, VerifyStaticToken(key, "cosmos", "so100", tok, now))
	require.True(t, VerifyStaticToken(key, "cosmos", "so100", tok, now.Add(4*time.Minute)))
	// Expired.
	require.False(t, VerifyStaticToken(key, "cosmos", "so100", tok, now.Add(6*time.Minute)))
	// Wrong scope.
	require.False(t, VerifyStaticToken(key, "other", "so100", tok, now))
	require.False(t, VerifyStaticToken(key, "cosmos", "other", tok, now))
	// Wrong key.
	require.False(t, VerifyStaticToken(DeriveStaticKey([]byte("other-seed")), "cosmos", "so100", tok, now))
}

func TestStaticToken_Malformed(t *testing.T) {
	key := DeriveStaticKey([]byte("seed"))
	now := time.Unix(1_700_000_000, 0)
	for _, tok := range []string{"", "noseparator", "notanumber.abcd", "123", "9999999999."} {
		require.False(t, VerifyStaticToken(key, "r", "m", tok, now), "token %q", tok)
	}
	// Tampered expiry keeps the old MAC → must fail.
	tok := MintStaticToken(key, "r", "m", now.Add(time.Minute))
	_, mac, _ := strings.Cut(tok, ".")
	require.False(t, VerifyStaticToken(key, "r", "m", "9999999999."+mac, now))
	// Nil key never verifies.
	require.False(t, VerifyStaticToken(nil, "r", "m", tok, now))
}
