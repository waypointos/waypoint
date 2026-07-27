// Package stamp captures dual capture-time stamps: wall clock plus
// CLOCK_MONOTONIC since boot plus the kernel boot id.
package stamp

import (
	"os"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/timestamppb"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

var (
	bootOnce sync.Once
	bootID   string
)

// BootID returns the kernel boot id, or "" where unavailable (non-Linux dev
// hosts). "" is the N/A value; consumers treat it as absent.
func BootID() string {
	bootOnce.Do(func() {
		b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
		if err != nil {
			return
		}
		bootID = strings.TrimSpace(string(b))
	})
	return bootID
}

// MonoNs returns CLOCK_MONOTONIC as nanoseconds since boot, 0 on failure.
func MonoNs() uint64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return 0
	}
	return uint64(ts.Sec)*1_000_000_000 + uint64(ts.Nsec)
}

// Now captures a dual stamp at the moment of call.
func Now() *waypointv1.Stamp {
	return &waypointv1.Stamp{
		T:      timestamppb.Now(),
		MonoNs: MonoNs(),
		BootId: BootID(),
	}
}
