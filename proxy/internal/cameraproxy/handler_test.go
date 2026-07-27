package cameraproxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// stubConns satisfies the connection-provider interface by handing back one
// fixed connection for any rover — enough for these RPC-plumbing tests.
type stubConns struct{ nc *nats.Conn }

func (s stubConns) Conn(string) (*nats.Conn, error) { return s.nc, nil }

func TestProxy_TunnelsOfferAndAnswer(t *testing.T) {
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	// Fake rover responds to rpc.camera_offer with a fixed answer.
	if _, err := nc.Subscribe("waypoint.sim-01.rpc.camera_offer", func(msg *nats.Msg) {
		var req waypointv1.CameraOfferRequest
		_ = proto.Unmarshal(msg.Data, &req)
		resp := &waypointv1.CameraOfferResponse{AnswerSdp: "v=0\ns=answer-for-" + req.Camera}
		body, _ := proto.Marshal(resp)
		_ = msg.Respond(body)
	}); err != nil {
		t.Fatal(err)
	}

	// Register handler routes directly on a mux to bypass auth — this test is
	// about NATS RPC plumbing, not session enforcement. Production wiring goes
	// through router.Router.RequireUser inside New().
	h := &Handler{deps: Deps{Conns: stubConns{nc}, RPCTimeout: 3 * time.Second}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rover/{id}/camera/{name}/whep", h.postOffer)
	mux.HandleFunc("DELETE /rover/{id}/camera/{name}/whep/{session}", h.deleteSession)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/rover/sim-01/camera/chassis-front/whep",
		bytes.NewReader([]byte("v=0\no=offer-from-test\n")))
	req.Header.Set("Content-Type", "application/sdp")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "/rover/sim-01/camera/chassis-front/whep/") {
		t.Fatalf("bad Location: %q", loc)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "answer-for-chassis-front") {
		t.Fatalf("body did not echo fake answer: %q", body)
	}
}

func TestProxy_TimeoutWhenRoverOffline(t *testing.T) {
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	h := &Handler{deps: Deps{Conns: stubConns{nc}, RPCTimeout: 200 * time.Millisecond}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rover/{id}/camera/{name}/whep", h.postOffer)
	mux.HandleFunc("DELETE /rover/{id}/camera/{name}/whep/{session}", h.deleteSession)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/rover/offline/camera/chassis-front/whep",
		"application/sdp", strings.NewReader("v=0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", resp.StatusCode)
	}
}
