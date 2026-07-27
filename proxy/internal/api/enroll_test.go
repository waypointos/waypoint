package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/operator"
)

func TestEnroll_AdminMintsIdentityTOML(t *testing.T) {
	op, _ := operator.New()
	fake := &fakeRoverCreator{}
	fakeHub := &fakeHub{}

	h := newEnrollHandler(fake, op, fakeHub, "wss://proxy.example.com/leaf", nil)

	body, _ := json.Marshal(map[string]string{
		"id":   "sim-01",
		"name": "Sim",
	})
	req := httptest.NewRequest("POST", "/api/rovers/enroll", bytes.NewReader(body))
	req = req.WithContext(authkit.WithUser(context.Background(),
		&authkit.CurrentUser{ID: uuid.New(), Email: "a@e.com", IsAdmin: true}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		IdentityTOML string `json:"identityToml"`
		RoverID      string `json:"roverId"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.RoverID != "sim-01" {
		t.Fatal()
	}
	for _, want := range []string{
		`[rover]`, `id   = "sim-01"`, `name = "Sim"`,
		`[proxy]`, `url             = "wss://proxy.example.com/leaf"`,
		`operator_pubkey =`,
		`[nats]`, `user_jwt    =`, `user_seed   =`,
		`[local]`, `bootstrap_admin_token =`,
	} {
		if !strings.Contains(resp.IdentityTOML, want) {
			t.Fatalf("expected %q in identity.toml, got:\n%s", want, resp.IdentityTOML)
		}
	}
	if fake.created != "sim-01" {
		t.Fatal("rover not persisted")
	}
	// Verify both credentials made it into persistence. Without them, a
	// proxy restart would orphan the rover (cf. migration 0007 + the
	// restoreRoverAccounts function in proxy/cmd/waypoint-proxy/main.go).
	if fake.accountJWT == "" {
		t.Fatal("account JWT not persisted")
	}
	if fake.accountSeed == "" {
		t.Fatal("account NKey seed not persisted")
	}
	if !fakeHub.registered {
		t.Fatal("account not registered with hub")
	}
}

type fakeRoverCreator struct {
	created     string
	accountJWT  string
	accountSeed string
	// Snapshot of what List() will return after Create() runs. Lets the
	// test verify RefreshSessionsAccount sees the freshly-enrolled row.
	row *db.Rover
}

func (f *fakeRoverCreator) Create(_ context.Context, id, _, pubkey, accountJWT, accountSeed string, _ uuid.UUID) (*db.Rover, error) {
	f.created = id
	f.accountJWT = accountJWT
	f.accountSeed = accountSeed
	f.row = &db.Rover{ID: id, AccountPubKey: pubkey, AccountJWT: &accountJWT, AccountSeed: &accountSeed}
	return f.row, nil
}

func (f *fakeRoverCreator) List(_ context.Context) ([]db.Rover, error) {
	if f.row == nil {
		return nil, nil
	}
	return []db.Rover{*f.row}, nil
}

type fakeHub struct {
	registered    bool
	registerCalls int
}

func (f *fakeHub) RegisterAccount(_ string) error {
	f.registered = true
	f.registerCalls++
	return nil
}
