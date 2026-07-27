package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/operator"
)

func TestDeleteRover_RevokesNATSBeforeDB(t *testing.T) {
	op, _ := operator.New()
	// Pre-mint the rover account so we have a realistic pubkey.
	_, accountPK, _, err := op.MintAccountJWT("sim-01", []string{"waypoint.sim-01.>"})
	if err != nil {
		t.Fatalf("mint account: %v", err)
	}

	rovers := &fakeRoverGetterDeleter{
		row: &db.Rover{ID: "sim-01", Name: "Sim", AccountPubKey: accountPK},
	}
	hub := &fakeHubRevoker{}

	h := newDeleteRoverHandler(rovers, op, hub, nil /* nc */, nil /* conns */, nil /* rpc */)

	req := httptest.NewRequest("DELETE", "/api/rovers/sim-01", nil)
	req.SetPathValue("id", "sim-01")
	req = req.WithContext(authkit.WithUser(context.Background(),
		&authkit.CurrentUser{ID: uuid.New(), Email: "a@e.com", IsAdmin: true}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if hub.revokedPK != accountPK {
		t.Fatalf("hub not revoked: got %q want %q", hub.revokedPK, accountPK)
	}
	if !rovers.deleted {
		t.Fatal("DB delete not called")
	}
	if hub.revokedAt > rovers.deletedAt {
		t.Fatal("DB delete happened before NATS revoke")
	}
}

func TestDeleteRover_NotFound(t *testing.T) {
	op, _ := operator.New()
	rovers := &fakeRoverGetterDeleter{getErr: pgx.ErrNoRows}
	hub := &fakeHubRevoker{}

	h := newDeleteRoverHandler(rovers, op, hub, nil, nil, nil)
	req := httptest.NewRequest("DELETE", "/api/rovers/missing", nil)
	req.SetPathValue("id", "missing")
	req = req.WithContext(authkit.WithUser(context.Background(),
		&authkit.CurrentUser{ID: uuid.New(), Email: "a@e.com", IsAdmin: true}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	if hub.revokedPK != "" {
		t.Fatal("hub revoke called for missing rover")
	}
}

func TestDeleteRover_RevokeFailureAbortsDelete(t *testing.T) {
	op, _ := operator.New()
	_, accountPK, _, _ := op.MintAccountJWT("sim-02", []string{"waypoint.sim-02.>"})

	rovers := &fakeRoverGetterDeleter{
		row: &db.Rover{ID: "sim-02", Name: "Sim2", AccountPubKey: accountPK},
	}
	hub := &fakeHubRevoker{err: errors.New("hub down")}

	h := newDeleteRoverHandler(rovers, op, hub, nil, nil, nil)
	req := httptest.NewRequest("DELETE", "/api/rovers/sim-02", nil)
	req.SetPathValue("id", "sim-02")
	req = req.WithContext(authkit.WithUser(context.Background(),
		&authkit.CurrentUser{ID: uuid.New(), Email: "a@e.com", IsAdmin: true}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if rovers.deleted {
		t.Fatal("DB delete should not run after revoke failure")
	}
}

// --- fakes ----------------------------------------------------------------

type fakeRoverGetterDeleter struct {
	row       *db.Rover
	getErr    error
	deleted   bool
	deletedAt int
}

var monotonic int

func (f *fakeRoverGetterDeleter) Get(_ context.Context, _ string) (*db.Rover, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.row, nil
}
func (f *fakeRoverGetterDeleter) Delete(_ context.Context, _ string) error {
	f.deleted = true
	monotonic++
	f.deletedAt = monotonic
	return nil
}

type fakeHubRevoker struct {
	revokedPK string
	revokedAt int
	err       error
}

func (f *fakeHubRevoker) RevokeAccount(pk, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.revokedPK = pk
	monotonic++
	f.revokedAt = monotonic
	return nil
}
