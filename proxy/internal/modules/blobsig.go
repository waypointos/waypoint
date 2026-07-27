// HMAC signing for the blob-download URLs pushed to rovers. The desired-state
// set travels over the rover's own authenticated NATS connection, so a URL
// carrying a valid signature is proof the proxy issued it for that rover —
// letting the raw endpoint skip session auth entirely.
package modules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// DeriveBlobKey turns a long-lived deployment secret (the NATS operator seed)
// into the dedicated key for blob-URL signing. Domain separation keeps this
// use independent of the seed's other derivations.
func DeriveBlobKey(secret []byte) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("waypoint/module-blob-url/v1"))
	return m.Sum(nil)
}

func signBlobPath(key []byte, roverID, moduleID, version string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(roverID + "\x00" + moduleID + "\x00" + version))
	return hex.EncodeToString(m.Sum(nil))
}

// VerifyBlobSig reports whether sig authorizes downloading moduleID@version
// as roverID. Constant-time; a nil key never verifies.
func VerifyBlobSig(key []byte, roverID, moduleID, version, sig string) bool {
	if len(key) == 0 || sig == "" {
		return false
	}
	return hmac.Equal([]byte(signBlobPath(key, roverID, moduleID, version)), []byte(sig))
}
