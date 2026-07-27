//go:build linux

package clockanchor

import "golang.org/x/sys/unix"

// NTPSynced reports whether the kernel clock is NTP-disciplined. adjtimex
// returns TIME_ERROR while the clock is unsynchronized.
func NTPSynced() bool {
	var tx unix.Timex
	st, err := unix.Adjtimex(&tx)
	return err == nil && st != unix.TIME_ERROR
}
