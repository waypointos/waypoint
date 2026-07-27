package image

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

const publishInterval = 60 * time.Second

type Publisher struct {
	nc      *nats.Conn
	roverID string
	loader  *Loader
}

func NewPublisher(nc *nats.Conn, roverID string, loader *Loader) *Publisher {
	if loader == nil {
		loader = DefaultLoader()
	}
	return &Publisher{nc: nc, roverID: roverID, loader: loader}
}

// Start publishes an ImageState immediately, then every publishInterval until
// ctx is cancelled. Returns an error if the immediate publish fails.
func (p *Publisher) Start(ctx context.Context) error {
	if err := p.publishOnce(); err != nil {
		return err
	}
	go p.loop(ctx)
	return nil
}

func (p *Publisher) loop(ctx context.Context) {
	t := time.NewTicker(publishInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = p.publishOnce()
		}
	}
}

func (p *Publisher) publishOnce() error {
	s, err := p.loader.Load()
	if err != nil {
		return err
	}
	msg := &waypointv1.ImageState{
		Version:   s.Version,
		Variant:   s.Variant,
		Partition: s.Partition,
		Bootcount: int32(s.Bootcount),
		BuiltAt:   s.BuiltAt,
	}
	body, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	subj := fmt.Sprintf("waypoint.%s.infra.system.image", p.roverID)
	return p.nc.Publish(subj, body)
}
