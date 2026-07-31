//go:build linux

package led

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Linux SPI ioctls, from linux/spi/spidev.h (_IOW('k', nr, size)).
const (
	spiIocWrMode       = 0x40016b01 // u8
	spiIocWrBitsPer    = 0x40016b03 // u8
	spiIocWrMaxSpeedHz = 0x40046b04 // u32
)

type spiDriver struct {
	f *os.File
}

// OpenSPI opens the spidev device configured for WS2812 bit streaming.
func OpenSPI(device string) (Driver, error) {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("led: open %s: %w", device, err)
	}
	mode := uint8(0)
	bits := uint8(8)
	speed := uint32(SPISpeedHz)
	for _, c := range []struct {
		req uintptr
		ptr unsafe.Pointer
	}{
		{spiIocWrMode, unsafe.Pointer(&mode)},
		{spiIocWrBitsPer, unsafe.Pointer(&bits)},
		{spiIocWrMaxSpeedHz, unsafe.Pointer(&speed)},
	} {
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), c.req, uintptr(c.ptr)); errno != 0 {
			f.Close()
			return nil, fmt.Errorf("led: ioctl %s: %v", device, errno)
		}
	}
	return &spiDriver{f: f}, nil
}

func (d *spiDriver) Frame(pixels []RGB) error {
	_, err := d.f.Write(Encode(pixels))
	return err
}

func (d *spiDriver) Close() error { return d.f.Close() }
