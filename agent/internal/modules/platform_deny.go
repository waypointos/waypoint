package modules

import (
	"log/slog"

	"github.com/waypointos/waypoint/protocol/platform/descriptor"
)

// defaultDenyIDs is the pre-descriptor hardcoded drive-wheel set, kept as the
// fallback for images that do not bake a platform descriptor.
var defaultDenyIDs = map[uint32]struct{}{7: {}, 8: {}, 9: {}, 10: {}}

// loadDenyIDs derives the module-surface deny-list from the first readable
// platform descriptor path. Missing descriptor falls back to the hardcoded
// set: failing open on module servo access is not acceptable, failing the
// agent for a missing file is too brittle during the image transition.
func loadDenyIDs(paths []string) map[uint32]struct{} {
	for _, p := range paths {
		if p == "" {
			continue
		}
		d, err := descriptor.Load(p)
		if err != nil {
			continue
		}
		deny := map[uint32]struct{}{}
		for _, id := range d.PlatformOwnedBusIDs() {
			deny[id] = struct{}{}
		}
		slog.Info("servo deny-list from platform descriptor", "path", p, "ids", len(deny))
		return deny
	}
	slog.Warn("platform descriptor not found; using hardcoded servo deny-list")
	return defaultDenyIDs
}
