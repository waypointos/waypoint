package cameras

import (
	"fmt"
	"sort"
	"strconv"
)

// assembleSpecs turns a set of capture-capable device paths into auto-named
// camera specs, ordered by their numeric /dev/videoN index (so video2 sorts
// before video10). The name is derived from that index (camera-<N>), not the
// position in the list, so a camera's name stays stable when a sibling is
// plugged or unplugged — otherwise hot-swapping one would renumber the others
// and yank the tile/WHEP URL out from under a working feed. Pipeline is left
// empty so pipeline.Pick resolves it by OS (pi5 on the rover).
func assembleSpecs(paths []string) []CameraSpec {
	sorted := append([]string(nil), paths...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return deviceIndex(sorted[i]) < deviceIndex(sorted[j])
	})
	specs := make([]CameraSpec, 0, len(sorted))
	for _, p := range sorted {
		specs = append(specs, CameraSpec{Name: fmt.Sprintf("camera-%d", deviceIndex(p)), Device: p})
	}
	return specs
}

// deviceIndex extracts the trailing integer of a /dev/videoN path. Paths
// without a trailing number sort last (large sentinel).
func deviceIndex(path string) int {
	i := len(path)
	for i > 0 && path[i-1] >= '0' && path[i-1] <= '9' {
		i--
	}
	n, err := strconv.Atoi(path[i:])
	if err != nil {
		return 1 << 30
	}
	return n
}
