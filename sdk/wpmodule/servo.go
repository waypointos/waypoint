package wpmodule

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// ServoClient speaks the agent's servo-control broker subjects. The module
// must declare requires = ["servo-control"]; the agent relays to core and
// enforces the platform-owned deny-list.
type ServoClient struct{ m *M }

func (m *M) Servo() *ServoClient { return &ServoClient{m: m} }

func (s *ServoClient) publish(c *waypointv1.ServoControl) error {
	b, err := proto.Marshal(c)
	if err != nil {
		return err
	}
	return s.m.Publish(s.m.Subject("servo.cmd"), b)
}

func (s *ServoClient) SetMode(id, mode uint32) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetMode{SetMode: mode}})
}

func (s *ServoClient) SetTorqueEnable(id uint32, on bool) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetTorqueEnable{SetTorqueEnable: on}})
}

func (s *ServoClient) SetGoalPosition(id, raw uint32) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetGoalPosition{SetGoalPosition: raw}})
}

func (s *ServoClient) SetTorqueLimit(id, raw uint32) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetTorqueLimit{SetTorqueLimit: raw}})
}

// SetAngleLimits writes the EEPROM min/max travel for a servo.
func (s *ServoClient) SetAngleLimits(id, minRaw, maxRaw uint32) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetAngleLimits{
		SetAngleLimits: &waypointv1.AngleLimits{MinRaw: minRaw, MaxRaw: maxRaw},
	}})
}

func (s *ServoClient) SetOvercurrentLimit(id, raw uint32) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetOvercurrentLimit{SetOvercurrentLimit: raw}})
}

// SetGoalSpeed caps the position-mode moving speed (raw steps/s; 0 = max). Use a
// low value during calibration so the joint creeps onto its hard stop instead of
// slamming it at full speed.
func (s *ServoClient) SetGoalSpeed(id, raw uint32) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetGoalSpeed{SetGoalSpeed: raw}})
}

// SetGoalVelocity drives a wheel-mode servo at a signed raw velocity (ticks/s),
// unlike SetGoalSpeed which only caps the unsigned position-mode moving speed.
func (s *ServoClient) SetGoalVelocity(id uint32, raw int32) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetGoalVelocity{SetGoalVelocity: raw}})
}

// Tuning carries one-time servo control-loop parameters. Nil fields are left
// unchanged on the servo.
type Tuning struct {
	PCoefficient    *uint32
	ICoefficient    *uint32
	DCoefficient    *uint32
	Acceleration    *uint32
	MaxAcceleration *uint32
	ReturnDelay     *uint32
}

// SetTuning applies one-time control-loop tuning (PID gains, accel, bus delay).
// Apply at startup with torque off, not per tick.
func (s *ServoClient) SetTuning(id uint32, t Tuning) error {
	return s.publish(&waypointv1.ServoControl{ServoId: id, Op: &waypointv1.ServoControl_SetTuning{
		SetTuning: &waypointv1.ServoTuning{
			PCoefficient:    t.PCoefficient,
			ICoefficient:    t.ICoefficient,
			DCoefficient:    t.DCoefficient,
			Acceleration:    t.Acceleration,
			MaxAcceleration: t.MaxAcceleration,
			ReturnDelay:     t.ReturnDelay,
		},
	}})
}

// SyncWriteGoals sends one coordinated multi-servo goal write.
func (s *ServoClient) SyncWriteGoals(goals []*waypointv1.ServoGoal) error {
	b, err := proto.Marshal(&waypointv1.ServoSyncWrite{Goals: goals})
	if err != nil {
		return err
	}
	return s.m.Publish(s.m.Subject("servo.sync"), b)
}

// Read requests one servo's raw state through the broker.
func (s *ServoClient) Read(id uint32) (*waypointv1.ServoState, error) {
	b, err := proto.Marshal(&waypointv1.ServoReadRequest{ServoId: id})
	if err != nil {
		return nil, err
	}
	msg, err := s.m.Request(s.m.Subject("servo.read"), b, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("servo read %d: %w", id, err)
	}
	var st waypointv1.ServoState
	if err := proto.Unmarshal(msg.Data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}
