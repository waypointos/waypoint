// Command multi-component is a conformance fixture: one module serving a
// typed sensor component and a generic "probe" class side by side.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/sdk/wpmodule"
)

type demoSensor struct{}

func (demoSensor) State() *waypointv1.SensorReadings {
	return &waypointv1.SensorReadings{Readings: []*waypointv1.SensorReading{
		{Name: "bus_voltage", Value: proto.Float64(12.0), Unit: "V", Ok: true},
	}}
}

func main() {
	err := wpmodule.Run(context.Background(), wpmodule.Options{ID: "multi-component"}, func(m *wpmodule.M) error {
		if _, err := m.ServeSensor(demoSensor{}); err != nil {
			return err
		}
		// Generic classes have no SDK serve loop; publish probe.state directly.
		go func() {
			t := time.NewTicker(50 * time.Millisecond)
			defer t.Stop()
			subj := m.Subject("probe.state")
			for {
				select {
				case <-m.Done():
					return
				case <-t.C:
					_ = m.Publish(subj, []byte("tick"))
				}
			}
		}()
		return nil
	})
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
