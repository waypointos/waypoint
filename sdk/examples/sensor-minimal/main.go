// Command sensor-minimal is the smallest complete Waypoint module: a sensor
// component publishing two readings, one of them N/A, on the standard API.
package main

import (
	"context"
	"log/slog"
	"math"
	"os"
	"time"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/sdk/wpmodule"
)

type demoSensor struct{ start time.Time }

func (d demoSensor) State() *waypointv1.SensorReadings {
	v := 12.0 + 0.3*math.Sin(time.Since(d.start).Seconds())
	return &waypointv1.SensorReadings{Readings: []*waypointv1.SensorReading{
		{Name: "bus_voltage", Value: proto.Float64(v), Unit: "V", Ok: true},
		{Name: "water_depth", Unit: "m", Ok: false}, // N/A: sensor not fitted
	}}
}

func main() {
	err := wpmodule.Run(context.Background(), wpmodule.Options{ID: "sensor-minimal"}, func(m *wpmodule.M) error {
		_, err := m.ServeSensor(demoSensor{start: time.Now()})
		return err
	})
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
