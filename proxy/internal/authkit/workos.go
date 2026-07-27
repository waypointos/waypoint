package authkit

import (
	"context"
	"fmt"

	"github.com/workos/workos-go/v4/pkg/usermanagement"
)

type WorkOS struct {
	client      *usermanagement.Client
	clientID    string
	redirectURI string
}

// Authenticator is the subset of *WorkOS used by handlers.go. Tests inject a
// stub so callback flows can run without hitting the WorkOS API.
type Authenticator interface {
	AuthorizationURL(state string) string
	Authenticate(ctx context.Context, code string) (*AuthResult, error)
}

func New(apiKey, clientID, redirectURI string) *WorkOS {
	return &WorkOS{
		client:      usermanagement.NewClient(apiKey),
		clientID:    clientID,
		redirectURI: redirectURI,
	}
}

func (w *WorkOS) AuthorizationURL(state string) string {
	u, _ := w.client.GetAuthorizationURL(usermanagement.GetAuthorizationURLOpts{
		ClientID:    w.clientID,
		RedirectURI: w.redirectURI,
		Provider:    "authkit",
		State:       state,
	})
	return u.String()
}

type AuthResult struct {
	WorkOSUserID string
	Email        string
}

func (w *WorkOS) Authenticate(ctx context.Context, code string) (*AuthResult, error) {
	resp, err := w.client.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
		ClientID: w.clientID,
		Code:     code,
	})
	if err != nil {
		return nil, fmt.Errorf("workos authenticate: %w", err)
	}
	return &AuthResult{
		WorkOSUserID: resp.User.ID,
		Email:        resp.User.Email,
	}, nil
}

// InviteResult is what the admin invite endpoint returns to the caller.
// AcceptURL is the WorkOS-hosted accept-invitation URL — admins can share it
// out-of-band if the email never lands. EmailSent reports whether the upstream
// SDK promised to deliver an email (true for the WorkOS SendInvitation flow).
type InviteResult struct {
	AcceptURL string
	EmailSent bool
}

// SendInvitation triggers an email invitation via WorkOS. The WorkOS service
// is responsible for delivering the email; we surface the AcceptInvitationUrl
// so admins can hand-deliver the link if needed.
func (w *WorkOS) SendInvitation(ctx context.Context, email string, inviterUserID string) (InviteResult, error) {
	opts := usermanagement.SendInvitationOpts{Email: email}
	if inviterUserID != "" {
		opts.InviterUserID = inviterUserID
	}
	inv, err := w.client.SendInvitation(ctx, opts)
	if err != nil {
		return InviteResult{}, fmt.Errorf("workos invite: %w", err)
	}
	return InviteResult{
		AcceptURL: inv.AcceptInvitationUrl,
		EmailSent: true,
	}, nil
}
