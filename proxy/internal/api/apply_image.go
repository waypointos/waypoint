package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// imageRecorder narrows the DB dependency so tests can substitute a fake.
type imageRecorder interface {
	RecordApply(ctx context.Context, roverID, url string, version *string, user uuid.UUID) (*db.ApplyRecord, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

// natsRequester is the subset of *nats.Conn used here, so tests can inject a
// stub (e.g. one that returns nats.ErrTimeout without waiting).
type natsRequester interface {
	Request(subj string, data []byte, timeout time.Duration) (*nats.Msg, error)
}

const applyImageTimeout = 10 * time.Second

func newApplyImageHandler(images imageRecorder, requesterFor func(roverID string) (natsRequester, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := authkit.UserFrom(r.Context())
		roverID := r.PathValue("id")

		var body struct {
			URL            string `json:"url"`
			ExpectedSHA256 string `json:"expectedSha256"`
			Version        string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var versionPtr *string
		if body.Version != "" {
			v := body.Version
			versionPtr = &v
		}

		rec, err := images.RecordApply(r.Context(), roverID, body.URL, versionPtr, u.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		req := &waypointv1.ApplyImageRequest{Url: body.URL}
		if versionPtr != nil {
			req.Version = versionPtr
		}
		if body.ExpectedSHA256 != "" {
			sha := body.ExpectedSHA256
			req.ExpectedSha256 = &sha
		}
		reqBytes, _ := proto.Marshal(req)

		subj := "waypoint." + roverID + ".rpc.apply_image"
		nc, err := requesterFor(roverID)
		if err != nil {
			_ = images.UpdateStatus(context.Background(), rec.ID, "error")
			http.Error(w, "rover unreachable: "+err.Error(), http.StatusGatewayTimeout)
			return
		}
		msg, err := nc.Request(subj, reqBytes, applyImageTimeout)
		if err != nil {
			// Use background ctx so we still record the terminal status even if
			// the inbound request context is being cancelled by the timeout.
			_ = images.UpdateStatus(context.Background(), rec.ID, "error")
			http.Error(w, "rover unreachable: "+err.Error(), http.StatusGatewayTimeout)
			return
		}

		var resp waypointv1.ApplyImageResponse
		_ = proto.Unmarshal(msg.Data, &resp)
		_ = images.UpdateStatus(context.Background(), rec.ID, resp.Status)
		if resp.Status != "accepted" {
			errMsg := ""
			if resp.Error != nil {
				errMsg = *resp.Error
			}
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"applyId": rec.ID.String(),
			"status":  "accepted",
		})
	})
}
