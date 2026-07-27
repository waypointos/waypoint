package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
)

func TestGrantAccess_Admin(t *testing.T) {
	fake := &fakeAccessWriter{}
	h := newGrantHandler(fake)

	body, _ := json.Marshal(map[string]string{"userId": uuid.New().String(), "role": "control"})
	req := httptest.NewRequest("POST", "/api/rovers/sim-01/access", bytes.NewReader(body))
	req.SetPathValue("id", "sim-01")
	req = req.WithContext(authkit.WithUser(context.Background(),
		&authkit.CurrentUser{ID: uuid.New(), IsAdmin: true}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if fake.granted != "sim-01" {
		t.Fatal("grant not called")
	}
}

type fakeAccessWriter struct{ granted string }

func (f *fakeAccessWriter) Grant(_ context.Context, _ uuid.UUID, rover, _ string, _ uuid.UUID) error {
	f.granted = rover
	return nil
}

func (f *fakeAccessWriter) Revoke(_ context.Context, _ uuid.UUID, _ string) error { return nil }
