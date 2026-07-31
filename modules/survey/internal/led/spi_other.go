//go:build !linux

package led

import "fmt"

// OpenSPI is Linux-only; other hosts fall back to the log driver.
func OpenSPI(device string) (Driver, error) {
	return nil, fmt.Errorf("led: spidev unsupported on this OS")
}
