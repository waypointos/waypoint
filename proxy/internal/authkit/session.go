package authkit

import (
	"errors"
	"net/http"

	"github.com/gorilla/securecookie"
)

type Session struct {
	UserID string `json:"u"`
}

const sessionCookie = "wp_session"

type SessionCodec struct {
	sc     *securecookie.SecureCookie
	secure bool
}

func NewSessionCodec(secret []byte, secure bool) *SessionCodec {
	// First 32 bytes hash key, next 32 (if available) encryption key.
	hashKey := secret
	var blockKey []byte
	if len(secret) >= 64 {
		hashKey = secret[:32]
		blockKey = secret[32:64]
	} else if len(secret) >= 32 {
		blockKey = secret[:32]
	}
	return &SessionCodec{sc: securecookie.New(hashKey, blockKey), secure: secure}
}

func (c *SessionCodec) Set(w http.ResponseWriter, s Session) error {
	enc, err := c.sc.Encode(sessionCookie, s)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    enc,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600, // 30 days
	})
	return nil
}

func (c *SessionCodec) Get(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, errors.New("no session cookie")
	}
	var s Session
	if err := c.sc.Decode(sessionCookie, cookie.Value, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *SessionCodec) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookie,
		Path:   "/",
		Secure: c.secure,
		MaxAge: -1,
	})
}
