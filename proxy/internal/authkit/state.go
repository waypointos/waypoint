package authkit

import (
	"net/http"

	"github.com/gorilla/securecookie"
)

const stateCookieName = "wp_state"

// StatePayload is what we stash in the wp_state cookie. State is the OAuth
// CSRF token echoed back by WorkOS; Next is the validated post-login redirect.
type StatePayload struct {
	State string `json:"s"`
	Next  string `json:"n"`
}

type StateCodec struct {
	sc     *securecookie.SecureCookie
	secure bool
}

func NewStateCodec(secret []byte, secure bool) *StateCodec {
	hashKey := secret
	var blockKey []byte
	if len(secret) >= 64 {
		hashKey = secret[:32]
		blockKey = secret[32:64]
	} else if len(secret) >= 32 {
		blockKey = secret[:32]
	}
	return &StateCodec{sc: securecookie.New(hashKey, blockKey), secure: secure}
}

func (c *StateCodec) Set(w http.ResponseWriter, p StatePayload) error {
	enc, err := c.sc.Encode(stateCookieName, p)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    enc,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	return nil
}

func (c *StateCodec) Decode(value string) (*StatePayload, error) {
	var p StatePayload
	if err := c.sc.Decode(stateCookieName, value, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *StateCodec) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   stateCookieName,
		Path:   "/auth",
		Secure: c.secure,
		MaxAge: -1,
	})
}
