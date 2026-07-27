package waypointv1_test

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

func TestDriveCommandRoundtrip(t *testing.T) {
	v := 0.42
	yaw := -0.18
	in := &waypointv1.DriveCommand{
		T:            timestamppb.New(time.Unix(1700000000, 0)),
		BodyVxMps:    &v,
		YawRateRadps: &yaw,
	}

	bs, err := proto.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out := &waypointv1.DriveCommand{}
	if err := proto.Unmarshal(bs, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.GetBodyVxMps() != v || out.GetYawRateRadps() != yaw {
		t.Fatalf("roundtrip mismatch: got %+v", out)
	}
}

func TestNAFieldsAreOptional(t *testing.T) {
	// Power message with no battery_percent set should round-trip with the
	// field still absent — N/A is preserved across the wire.
	in := &waypointv1.PowerTelemetry{T: timestamppb.Now()}
	bs, err := proto.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := &waypointv1.PowerTelemetry{}
	if err := proto.Unmarshal(bs, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.BatteryPercent != nil {
		t.Fatalf("battery_percent should be nil (N/A), got %v", *out.BatteryPercent)
	}
}
