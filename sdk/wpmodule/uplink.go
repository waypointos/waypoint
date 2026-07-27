package wpmodule

import (
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// PublishUplink feeds the uplink rail (module.<id>.uplink); the agent mirrors
// it to telemetry.uplink for modules that declare provides = ["uplink"].
func (m *M) PublishUplink(u *waypointv1.UplinkTelemetry) error {
	b, err := proto.Marshal(u)
	if err != nil {
		return err
	}
	return m.Publish(m.Subject("uplink"), b)
}
