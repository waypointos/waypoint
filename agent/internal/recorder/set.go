package recorder

import (
	"fmt"
	"sort"

	"github.com/waypointos/waypoint/protocol/platform/descriptor"
)

// StreamSpec names one recorded stream: a rover-relative subject and its
// protobuf message full name.
type StreamSpec struct {
	Subject string
	Message string
}

// altitudeCommands maps an [actions] altitude to the command stream it
// implies. Grows with new altitudes; an unknown altitude is skipped.
var altitudeCommands = map[string]StreamSpec{
	"body_twist":     {Subject: "cmd.drive", Message: "waypoint.v1.DriveCommand"},
	"joint_position": {Subject: "cmd.servo", Message: "waypoint.v1.ServoControl"},
}

// componentStreams lists the sandbox leaves recorded per component class
// (subject pattern parameterized by module id).
var componentStreams = map[string][]StreamSpec{
	"arm": {
		{Subject: "module.%s.arm.state", Message: "waypoint.v1.ArmState"},
		{Subject: "module.%s.arm.cmd", Message: "waypoint.v1.ArmCommand"},
	},
	"sensor": {
		{Subject: "module.%s.sensor.state", Message: "waypoint.v1.SensorReadings"},
	},
	"base": {
		{Subject: "module.%s.base.state", Message: "waypoint.v1.BaseState"},
		{Subject: "module.%s.base.cmd", Message: "waypoint.v1.BaseCommand"},
	},
}

// ResolveSet derives the recording set: descriptor observation streams, the
// command streams implied by action altitudes, and the state/cmd leaves of
// every active component. Resolved at episode start, never hardcoded.
func ResolveSet(d *descriptor.Descriptor, components map[string]string) []StreamSpec {
	seen := map[string]bool{}
	var out []StreamSpec
	add := func(s StreamSpec) {
		if !seen[s.Subject] {
			seen[s.Subject] = true
			out = append(out, s)
		}
	}
	if d != nil {
		if d.Observations != nil {
			for _, st := range d.Observations.Streams {
				add(StreamSpec{Subject: st.Subject, Message: st.Message})
			}
		}
		if d.Actions != nil {
			for _, alt := range d.Actions.Altitudes {
				if spec, ok := altitudeCommands[alt]; ok {
					add(spec)
				}
			}
		}
	}
	ids := make([]string, 0, len(components))
	for id := range components {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		class := components[id]
		tpls, ok := componentStreams[class]
		if !ok {
			// Untyped class: record the generic leaves schemaless (empty Message).
			if class == "" {
				continue
			}
			tpls = []StreamSpec{
				{Subject: "module.%s." + class + ".state"},
				{Subject: "module.%s." + class + ".cmd"},
			}
		}
		for _, tpl := range tpls {
			add(StreamSpec{Subject: fmt.Sprintf(tpl.Subject, id), Message: tpl.Message})
		}
	}
	return out
}
