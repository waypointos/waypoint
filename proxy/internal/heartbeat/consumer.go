package heartbeat

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

type Consumer struct {
	nc     *nats.Conn
	rovers *db.RoversRepo
}

func Start(nc *nats.Conn, rovers *db.RoversRepo) (*Consumer, error) {
	c := &Consumer{nc: nc, rovers: rovers}
	if _, err := nc.Subscribe("waypoint.*.infra.heartbeat", c.onBeat); err != nil {
		return nil, err
	}
	if _, err := nc.Subscribe("waypoint.*.infra.system.image", c.onImage); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Consumer) onBeat(msg *nats.Msg) {
	parts := strings.Split(msg.Subject, ".")
	if len(parts) < 4 {
		return
	}
	roverID := parts[1]
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.rovers.UpdateLastSeen(ctx, roverID, time.Now()); err != nil {
		log.Printf("update last_seen %s: %v", roverID, err)
	}
}

func (c *Consumer) onImage(msg *nats.Msg) {
	parts := strings.Split(msg.Subject, ".")
	if len(parts) < 5 {
		return
	}
	roverID := parts[1]
	var state waypointv1.ImageState
	if err := proto.Unmarshal(msg.Data, &state); err != nil {
		log.Printf("image: decode %s: %v", roverID, err)
		return
	}
	if state.Version == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.rovers.UpdateImageVersion(ctx, roverID, state.Version); err != nil {
		log.Printf("update image_version %s: %v", roverID, err)
	}
}
