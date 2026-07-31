package modules

import "github.com/waypointos/waypoint/agent/internal/localauth"

// ToLocalAuthPermissions converts a manifest Permissions block to the
// localauth-package shape. localauth handles the rover-id substitution
// when AddModuleUser is called.
func ToLocalAuthPermissions(p Permissions) localauth.ModulePermissions {
	return localauth.ModulePermissions{
		Publish:   append([]string(nil), p.Publish...),
		Subscribe: append([]string(nil), p.Subscribe...),
	}
}

// ManifestAuthPermissions derives the localauth permissions for a module from
// its manifest: the declared [permissions] plus the standard leaves of every
// declared component. Only the baked-manifest mint path narrows to
// explicit subjects; the install paths grant the whole module subtree, which
// already covers component leaves.
func ManifestAuthPermissions(m *Manifest) localauth.ModulePermissions {
	p := ToLocalAuthPermissions(m.Permissions)
	for _, c := range m.Components {
		pub, sub := componentLeaves(m.Name, c.Class)
		p.Publish = append(p.Publish, pub...)
		p.Subscribe = append(p.Subscribe, sub...)
	}
	for _, r := range m.Requires {
		pub, sub := requiresLeaves(m.Name, r)
		p.Publish = append(p.Publish, pub...)
		p.Subscribe = append(p.Subscribe, sub...)
	}
	return p
}

// requiresLeaves returns the sandbox leaves the agent's brokers serve for a
// required capability, manifest wildcard form, so a baked manifest needs no
// explicit [permissions] entries for them.
func requiresLeaves(name, require string) (publish, subscribe []string) {
	base := "waypoint.*.module." + name + "."
	if require == "drive-control" {
		return []string{base + "drive.cmd"},
			[]string{base + "drive.telemetry", base + "mode"}
	}
	return nil, nil
}
