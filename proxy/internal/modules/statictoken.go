// HMAC tokens for module static-asset URLs. A dynamic import() cannot carry
// the admin session the way fetch() does (Safari drops it from module-script
// requests), so the dashboard mints a short-lived token over an authenticated
// call and appends it to the import URL instead.
package modules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// DeriveStaticKey turns a long-lived deployment secret (the NATS operator
// seed) into the dedicated key for static-URL tokens. Domain separation keeps
// this use independent of the seed's other derivations.
func DeriveStaticKey(secret []byte) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("waypoint/module-static-url/v1"))
	return m.Sum(nil)
}

func signStatic(key []byte, roverID, moduleID, exp string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(roverID + "\x00" + moduleID + "\x00" + exp))
	return hex.EncodeToString(m.Sum(nil))
}

// MintStaticToken issues a token authorizing static-asset reads for moduleID
// on roverID until expires.
func MintStaticToken(key []byte, roverID, moduleID string, expires time.Time) string {
	exp := strconv.FormatInt(expires.Unix(), 10)
	return exp + "." + signStatic(key, roverID, moduleID, exp)
}

// VerifyStaticToken reports whether token authorizes static-asset reads for
// moduleID on roverID at time now. Constant-time; a nil key never verifies.
func VerifyStaticToken(key []byte, roverID, moduleID, token string, now time.Time) bool {
	if len(key) == 0 || token == "" {
		return false
	}
	exp, mac, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || now.Unix() > unix {
		return false
	}
	return hmac.Equal([]byte(signStatic(key, roverID, moduleID, exp)), []byte(mac))
}
