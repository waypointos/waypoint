package modules

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/waypointos/waypoint/proxy/internal/db"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// roverConns yields a NATS connection in a rover's own account, so the desired
// set publish reaches the leaf-backed agent.
type roverConns interface {
	Conn(roverID string) (*nats.Conn, error)
}

// DesiredPusher assembles a rover's DesiredModuleSet from the registry and
// publishes it on waypoint.<rover-id>.modules.desired via the rover's own
// account connection.
type DesiredPusher struct {
	repo    *db.ModulesRepo
	conns   roverConns
	baseURL string // e.g. https://proxy.example/api/rovers
	blobKey []byte // signs raw_url so the rover can download without a session
}

func NewDesiredPusher(repo *db.ModulesRepo, conns roverConns, baseURL string, blobKey []byte) *DesiredPusher {
	return &DesiredPusher{repo: repo, conns: conns, baseURL: baseURL, blobKey: blobKey}
}

// Push assembles the DesiredModuleSet for one rover from the registry and
// publishes it on waypoint.<rover-id>.modules.desired.
func (p *DesiredPusher) Push(ctx context.Context, roverID string) error {
	rows, err := p.repo.DesiredForRover(ctx, roverID)
	if err != nil {
		return err
	}
	set := &waypointv1.DesiredModuleSet{
		T: timestamppb.New(time.Now().UTC()),
	}
	for _, row := range rows {
		if row.DesiredVersion == "" {
			continue
		}
		versions, _ := p.repo.ListVersions(ctx, row.ModuleID)
		var sha string
		for _, v := range versions {
			if v.Version == row.DesiredVersion {
				sha = v.RawSha256
				break
			}
		}
		sig := signBlobPath(p.blobKey, roverID, row.ModuleID, row.DesiredVersion)
		set.Modules = append(set.Modules, &waypointv1.ModuleDesired{
			Id:         row.ModuleID,
			Version:    row.DesiredVersion,
			RawUrl:     fmt.Sprintf("%s/%s/modules/%s/%s.raw?sig=%s", p.baseURL, roverID, row.ModuleID, row.DesiredVersion, sig),
			RawSha256:  sha,
			ConfigToml: row.ConfigTOML,
		})
	}
	bytes, err := proto.Marshal(set)
	if err != nil {
		return err
	}
	nc, err := p.conns.Conn(roverID)
	if err != nil {
		return err
	}
	return nc.Publish(fmt.Sprintf("waypoint.%s.modules.desired", roverID), bytes)
}
