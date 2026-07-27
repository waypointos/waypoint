//go:build linux && arm64

package cameras

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"
)

// VIDIOC_ENUM_FMT = _IOWR('V', 2, struct v4l2_fmtdesc); the struct is 64 bytes.
const vidiocEnumFmt = 0xc0405602

// V4L2_BUF_TYPE_VIDEO_CAPTURE.
const v4l2BufTypeVideoCapture = 1

// v4l2Fmtdesc mirrors struct v4l2_fmtdesc (64 bytes). Only index and typ are
// set on input; we just need to know whether the ioctl succeeds.
type v4l2Fmtdesc struct {
	index       uint32
	typ         uint32
	flags       uint32
	description [32]byte
	pixelformat uint32
	mbusCode    uint32
	reserved    [3]uint32
}

// discoverCameras enumerates /dev/video* and returns auto-named specs for the
// nodes that are real capture devices. Used when identity.toml declares no
// cameras.
func discoverCameras() []CameraSpec {
	nodes, _ := filepath.Glob("/dev/video*")
	capture := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if hasCaptureFormat(n) {
			capture = append(capture, n)
		}
	}
	specs := assembleSpecs(capture)
	slog.Info(fmt.Sprintf("cameras: auto-discovered %d capture device(s) from %d /dev/video node(s)", len(specs), len(nodes)))
	return specs
}

// hasCaptureFormat reports whether the V4L2 node enumerates at least one
// video-capture pixel format. UVC webcams expose a metadata node alongside the
// real capture node; the metadata node enumerates zero capture formats, so
// this filter skips it.
func hasCaptureFormat(path string) bool {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer unix.Close(fd)
	desc := v4l2Fmtdesc{typ: v4l2BufTypeVideoCapture}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(vidiocEnumFmt), uintptr(unsafe.Pointer(&desc)))
	return errno == 0
}
