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
// its manifest: the declared [permissions] plus the standard component leaves
// when [component] is declared. Only the baked-manifest mint path narrows to
// explicit subjects; the install paths grant the whole module subtree, which
// already covers component leaves.
func ManifestAuthPermissions(m *Manifest) localauth.ModulePermissions {
	p := ToLocalAuthPermissions(m.Permissions)
	if m.Component != nil {
		pub, sub := componentLeaves(m.Name, m.Component.Class)
		p.Publish = append(p.Publish, pub...)
		p.Subscribe = append(p.Subscribe, sub...)
	}
	return p
}
