// Command arm-sim serves the standard arm API over real (simulated) servos:
// it drives bus servos through the servo-control broker and reports joint
// state from servo reads. The in-tree conformance subject; pairs with the
// waypoint-bench platform (six module-owned servos on bus 1..6).
package main

import (
	"context"
	"log/slog"
	"os"
	"sync"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/sdk/wpmodule"
)

// Joint names in bus-id order 1..6, matching waypoint-bench.toml.
var jointNames = []string{"arm_1", "arm_2", "arm_3", "arm_4", "arm_5", "gripper"}

const ticksPerRev = 4096.0
const radPerTick = 2 * 3.141592653589793 / ticksPerRev
const centerTicks = 2048.0

func radToRaw(rad float64) uint32 {
	raw := centerTicks + rad/radPerTick
	if raw < 0 {
		raw = 0
	}
	if raw > 4095 {
		raw = 4095
	}
	return uint32(raw)
}

func rawToRad(raw uint32) float64 { return (float64(raw) - centerTicks) * radPerTick }

type simArm struct {
	mu sync.Mutex
	sv *wpmodule.ServoClient
}

func (a *simArm) State() *waypointv1.ArmState {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := &waypointv1.ArmState{}
	for i, name := range jointNames {
		j := &waypointv1.ArmJoint{Name: name, Calibrated: false}
		if s, err := a.sv.Read(uint32(i + 1)); err == nil && s.GetOk() {
			j.PositionRad = rawToRad(s.GetPositionRaw())
		}
		st.Joints = append(st.Joints, j)
	}
	return st
}

func (a *simArm) Command(cmd *waypointv1.ArmCommand) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cmd.GetStop() {
		// Halt: re-latch each joint's goal to its present position so any
		// in-flight move stops where it is and holds (torque stays as-is).
		for i := range jointNames {
			id := uint32(i + 1)
			s, err := a.sv.Read(id)
			if err != nil || !s.GetOk() {
				continue
			}
			if err := a.sv.SetGoalPosition(id, s.GetPositionRaw()); err != nil {
				return err
			}
		}
		return nil
	}
	g := cmd.GetGoals()
	if g == nil {
		return nil
	}
	for _, goal := range g.GetGoals() {
		for i, name := range jointNames {
			if name == goal.GetName() {
				id := uint32(i + 1)
				if err := a.sv.SetTorqueEnable(id, true); err != nil {
					return err
				}
				if err := a.sv.SetGoalPosition(id, radToRaw(goal.GetPositionRad())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func main() {
	err := wpmodule.Run(context.Background(), wpmodule.Options{ID: "arm-sim"}, func(m *wpmodule.M) error {
		_, err := m.ServeArm(&simArm{sv: m.Servo()})
		return err
	})
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
