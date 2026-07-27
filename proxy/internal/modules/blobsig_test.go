package modules

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlobSig_RoundTripAndTamper(t *testing.T) {
	key := DeriveBlobKey([]byte("operator-seed"))
	sig := signBlobPath(key, "rover-01", "so100", "0.1.1")

	require.True(t, VerifyBlobSig(key, "rover-01", "so100", "0.1.1", sig))

	require.False(t, VerifyBlobSig(key, "rover-02", "so100", "0.1.1", sig), "sig is rover-bound")
	require.False(t, VerifyBlobSig(key, "rover-01", "other", "0.1.1", sig), "sig is module-bound")
	require.False(t, VerifyBlobSig(key, "rover-01", "so100", "0.1.2", sig), "sig is version-bound")
	require.False(t, VerifyBlobSig(key, "rover-01", "so100", "0.1.1", ""), "empty sig never verifies")
	require.False(t, VerifyBlobSig(nil, "rover-01", "so100", "0.1.1", sig), "nil key never verifies")

	otherKey := DeriveBlobKey([]byte("different-seed"))
	require.False(t, VerifyBlobSig(otherKey, "rover-01", "so100", "0.1.1", sig))
}

// The field-separator must prevent ambiguous concatenations from colliding.
func TestBlobSig_NoFieldCollision(t *testing.T) {
	key := DeriveBlobKey([]byte("operator-seed"))
	a := signBlobPath(key, "rover-1", "x-mod", "1.0")
	b := signBlobPath(key, "rover-1-x", "mod", "1.0")
	require.NotEqual(t, a, b)
}
