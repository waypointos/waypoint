package image

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestPublisher_PublishesImageStateOnStart(t *testing.T) {
	s, err := server.NewServer(&server.Options{Port: -1})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("nats server not ready")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "image.toml")
	bootPath := filepath.Join(dir, "bootcount")
	contents := `[image]
version = "1.2.3"
variant = "prod"
partition = "A"
built_at = "2026-05-12T12:34:56Z"
`
	if err := os.WriteFile(tomlPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bootPath, []byte("7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := &Loader{ImageTOMLPath: tomlPath, BootcountPath: bootPath}

	got := make(chan []byte, 1)
	if _, err := nc.Subscribe("waypoint.test-rover.infra.system.image", func(msg *nats.Msg) {
		got <- msg.Data
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	pub := NewPublisher(nc, "test-rover", loader)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pub.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case payload := <-got:
		var msg waypointv1.ImageState
		if err := proto.Unmarshal(payload, &msg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if msg.Version != "1.2.3" || msg.Variant != "prod" || msg.Partition != "A" ||
			msg.Bootcount != 7 || msg.BuiltAt != "2026-05-12T12:34:56Z" {
			t.Fatalf("ImageState mismatch: %+v", &msg)
		}
	case <-time.After(time.Second):
		t.Fatal("no ImageState within 1s")
	}
}
