package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// fakeImages records calls and returns canned values.
type fakeImages struct {
	mu             sync.Mutex
	recordedRover  string
	recordedURL    string
	recordedVer    *string
	recordedUser   uuid.UUID
	recordedID     uuid.UUID
	statusCalls    []string
	recordApplyErr error
}

func (f *fakeImages) RecordApply(_ context.Context, roverID, url string, version *string, user uuid.UUID) (*db.ApplyRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recordApplyErr != nil {
		return nil, f.recordApplyErr
	}
	f.recordedRover = roverID
	f.recordedURL = url
	f.recordedVer = version
	f.recordedUser = user
	f.recordedID = uuid.New()
	return &db.ApplyRecord{ID: f.recordedID, RoverID: roverID, URL: url, Version: version, RequestedBy: user, Status: "pending"}, nil
}

func (f *fakeImages) UpdateStatus(_ context.Context, _ uuid.UUID, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusCalls = append(f.statusCalls, status)
	return nil
}

func (f *fakeImages) statuses() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.statusCalls))
	copy(out, f.statusCalls)
	return out
}

// stubRequester implements natsRequester without an embedded server. Used for
// fast simulation of timeouts.
type stubRequester struct {
	resp *nats.Msg
	err  error
}

func (s *stubRequester) Request(_ string, _ []byte, _ time.Duration) (*nats.Msg, error) {
	return s.resp, s.err
}

// startEmbeddedNATS spins up an in-process NATS server and returns a connected
// client. It registers t.Cleanup for shutdown.
func startEmbeddedNATS(t *testing.T) *nats.Conn {
	t.Helper()
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatalf("nats server: %v", err)
	}
	go s.Start()
	t.Cleanup(s.Shutdown)
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func newRequest(t *testing.T, roverID string, body any, user *authkit.CurrentUser) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	switch b := body.(type) {
	case string:
		buf.WriteString(b)
	default:
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest("POST", "/api/rovers/"+roverID+"/apply-image", &buf)
	req.SetPathValue("id", roverID)
	if user == nil {
		user = &authkit.CurrentUser{ID: uuid.New(), Email: "admin@example.com", IsAdmin: true}
	}
	req = req.WithContext(authkit.WithUser(context.Background(), user))
	return req
}

func TestApplyImage_AcceptedFlow(t *testing.T) {
	nc := startEmbeddedNATS(t)

	// Fake rover responder: assert the proto and reply with accepted.
	var gotURL, gotVersion string
	if _, err := nc.Subscribe("waypoint.sim-01.rpc.apply_image", func(msg *nats.Msg) {
		var req waypointv1.ApplyImageRequest
		_ = proto.Unmarshal(msg.Data, &req)
		gotURL = req.Url
		if req.Version != nil {
			gotVersion = *req.Version
		}
		body, _ := proto.Marshal(&waypointv1.ApplyImageResponse{Status: "accepted"})
		_ = msg.Respond(body)
	}); err != nil {
		t.Fatal(err)
	}

	fake := &fakeImages{}
	h := newApplyImageHandler(fake, func(string) (natsRequester, error) { return nc, nil })

	req := newRequest(t, "sim-01", map[string]string{
		"url":            "https://example.com/build.swu",
		"version":        "1.2.3",
		"expectedSha256": "deadbeef",
	}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "accepted" {
		t.Fatalf("status %q", resp["status"])
	}
	if _, err := uuid.Parse(resp["applyId"]); err != nil {
		t.Fatalf("applyId not a uuid: %q", resp["applyId"])
	}

	if fake.recordedRover != "sim-01" {
		t.Fatalf("recorded rover %q", fake.recordedRover)
	}
	if fake.recordedURL != "https://example.com/build.swu" {
		t.Fatalf("recorded url %q", fake.recordedURL)
	}
	if fake.recordedVer == nil || *fake.recordedVer != "1.2.3" {
		t.Fatalf("recorded version %v", fake.recordedVer)
	}
	if gotURL != "https://example.com/build.swu" {
		t.Fatalf("rover got url %q", gotURL)
	}
	if gotVersion != "1.2.3" {
		t.Fatalf("rover got version %q", gotVersion)
	}
	if statuses := fake.statuses(); len(statuses) != 1 || statuses[0] != "accepted" {
		t.Fatalf("status updates: %v", statuses)
	}
}

func TestApplyImage_RoverError(t *testing.T) {
	nc := startEmbeddedNATS(t)

	errMsg := "sha mismatch"
	if _, err := nc.Subscribe("waypoint.sim-01.rpc.apply_image", func(msg *nats.Msg) {
		body, _ := proto.Marshal(&waypointv1.ApplyImageResponse{Status: "error", Error: &errMsg})
		_ = msg.Respond(body)
	}); err != nil {
		t.Fatal(err)
	}

	fake := &fakeImages{}
	h := newApplyImageHandler(fake, func(string) (natsRequester, error) { return nc, nil })

	req := newRequest(t, "sim-01", map[string]string{"url": "https://example.com/build.swu"}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "sha mismatch") {
		t.Fatalf("body %q", rec.Body.String())
	}
	if statuses := fake.statuses(); len(statuses) != 1 || statuses[0] != "error" {
		t.Fatalf("status updates: %v", statuses)
	}
}

func TestApplyImage_RoverUnreachable(t *testing.T) {
	// Use a stub requester so the test does not wait for the real timeout.
	stub := &stubRequester{err: nats.ErrTimeout}
	fake := &fakeImages{}
	h := newApplyImageHandler(fake, func(string) (natsRequester, error) { return stub, nil })

	req := newRequest(t, "sim-01", map[string]string{"url": "https://example.com/build.swu"}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status %d", rec.Code)
	}
	if statuses := fake.statuses(); len(statuses) != 1 || statuses[0] != "error" {
		t.Fatalf("status updates: %v", statuses)
	}
}

func TestApplyImage_BadJSON(t *testing.T) {
	fake := &fakeImages{}
	h := newApplyImageHandler(fake, func(string) (natsRequester, error) { return &stubRequester{err: errors.New("should not be called")}, nil })

	req := newRequest(t, "sim-01", "not-json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	if len(fake.statuses()) != 0 || fake.recordedRover != "" {
		t.Fatalf("nothing should be recorded on bad json")
	}
}

func TestApplyImage_MissingURL(t *testing.T) {
	fake := &fakeImages{}
	h := newApplyImageHandler(fake, func(string) (natsRequester, error) { return &stubRequester{err: errors.New("should not be called")}, nil })

	req := newRequest(t, "sim-01", map[string]string{"version": "1.0.0"}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	if len(fake.statuses()) != 0 || fake.recordedRover != "" {
		t.Fatalf("nothing should be recorded on missing url")
	}
}
