package wpmodule

import (
	"time"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/protocol/platform/stamp"
)

// startStats publishes ModuleStats heartbeats until the returned stop func.
func (m *M) startStats(interval time.Duration) func() {
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		subj := m.Subject("stats")
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				st := &waypointv1.ModuleStats{
					Stamp:   stamp.Now(),
					UptimeS: uint64(time.Since(m.start).Seconds()),
				}
				if b, err := proto.Marshal(st); err == nil {
					_ = m.nc.Publish(subj, b)
				}
			}
		}
	}()
	return func() { close(stop) }
}
