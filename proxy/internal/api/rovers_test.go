package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
)

func TestListRovers_FiltersByAccess(t *testing.T) {
	fake := &fakeRovers{all: []db.Rover{{ID: "sim-01", Name: "Sim"}}}
	fakeAcc := &fakeAccess{grants: map[string]string{"sim-01": "control"}}
	h := newListHandler(fake, fakeAcc)

	req := httptest.NewRequest("GET", "/api/rovers", nil)
	user := &authkit.CurrentUser{ID: uuid.New(), Email: "u@e.com", IsAdmin: false}
	req = req.WithContext(authkit.WithUser(context.Background(), user))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["id"] != "sim-01" || got[0]["role"] != "control" {
		t.Fatalf("got %+v", got)
	}
}

type fakeRovers struct{ all []db.Rover }

func (f *fakeRovers) List(_ context.Context) ([]db.Rover, error) { return f.all, nil }

type fakeAccess struct{ grants map[string]string }

func (f *fakeAccess) ListForUser(_ context.Context, _ uuid.UUID) ([]db.Access, error) {
	var out []db.Access
	for r, role := range f.grants {
		out = append(out, db.Access{RoverID: r, Role: role})
	}
	return out, nil
}
