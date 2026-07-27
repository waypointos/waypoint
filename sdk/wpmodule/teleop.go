package wpmodule

import (
	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// TeleopInput subscribes the broker-relayed gamepad stream
// (module.<id>.input). Requires requires = ["teleop-input"].
func (m *M) TeleopInput(cb func(*waypointv1.GamepadSnapshot)) (*natsgo.Subscription, error) {
	return m.Subscribe(m.Subject("input"), func(msg *natsgo.Msg) {
		var s waypointv1.GamepadSnapshot
		if proto.Unmarshal(msg.Data, &s) != nil {
			return
		}
		cb(&s)
	})
}
