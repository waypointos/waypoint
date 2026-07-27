// Package partition identifies which rootfs slot (A or B) the rover is
// currently running from, by matching the kernel cmdline's root=PARTUUID
// against the slot UUIDs pinned at image-build time.
package partition

import (
	"fmt"
	"os"
	"strings"
)

// AUUID and BUUID are the rootfs-A / rootfs-B PARTUUIDs pinned in
// image/external/board/raspberrypi5/genimage.cfg. They are a build-time
// contract; changing them requires regenerating the image.
const (
	AUUID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	BUUID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

// Active reads cmdlinePath (normally /proc/cmdline) and returns "A" or "B"
// for the slot the running kernel booted from. Comparison is
// case-insensitive because the kernel doesn't normalize PARTUUID case.
func Active(cmdlinePath string) (string, error) {
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return "", err
	}
	cmdline := strings.ToLower(string(data))
	if strings.Contains(cmdline, "partuuid="+AUUID) {
		return "A", nil
	}
	if strings.Contains(cmdline, "partuuid="+BUUID) {
		return "B", nil
	}
	return "", fmt.Errorf("no known PARTUUID in cmdline: %s", strings.TrimSpace(string(data)))
}
