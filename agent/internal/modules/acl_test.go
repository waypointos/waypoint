package modules

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/waypointos/waypoint/agent/internal/localauth"
)

func TestToLocalAuthPermissions(t *testing.T) {
	perms := Permissions{
		Publish:   []string{"waypoint.*.module.umr.stats", "waypoint.*.module.umr.health.ready"},
		Subscribe: []string{"waypoint.*.cmd.umr"},
	}
	got := ToLocalAuthPermissions(perms)
	want := localauth.ModulePermissions{
		Publish:   []string{"waypoint.*.module.umr.stats", "waypoint.*.module.umr.health.ready"},
		Subscribe: []string{"waypoint.*.cmd.umr"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestManifestAuthPermissionsIncludeComponentLeaves(t *testing.T) {
	m := &Manifest{
		Name:       "myarm",
		Components: []Component{{Class: "arm", StateRateHz: 20}},
		Permissions: Permissions{
			Publish:   []string{"waypoint.*.module.myarm.stats"},
			Subscribe: []string{"waypoint.*.module.myarm.command"},
		},
	}
	p := ManifestAuthPermissions(m)
	assert.Contains(t, p.Publish, "waypoint.*.module.myarm.arm.state")
	assert.Contains(t, p.Publish, "waypoint.*.module.myarm.stats")
	assert.Contains(t, p.Subscribe, "waypoint.*.module.myarm.arm.cmd")
}

func TestManifestAuthPermissionsIncludeGenericClassLeaves(t *testing.T) {
	m := &Manifest{Name: "mydrill", Components: []Component{{Class: "drill", StateRateHz: 5}}}
	p := ManifestAuthPermissions(m)
	assert.Contains(t, p.Publish, "waypoint.*.module.mydrill.drill.state")
	assert.Contains(t, p.Subscribe, "waypoint.*.module.mydrill.drill.cmd")
}

func TestManifestAuthPermissionsSensorHasNoCmdLeaf(t *testing.T) {
	m := &Manifest{Name: "s", Components: []Component{{Class: "sensor", StateRateHz: 1}}}
	p := ManifestAuthPermissions(m)
	assert.Contains(t, p.Publish, "waypoint.*.module.s.sensor.state")
	assert.Empty(t, p.Subscribe)
}

func TestManifestAuthPermissionsUnionAcrossComponents(t *testing.T) {
	m := &Manifest{
		Name: "drill",
		Components: []Component{
			{Class: "drill", StateRateHz: 20},
			{Class: "sensor", StateRateHz: 10},
		},
	}
	p := ManifestAuthPermissions(m)
	assert.Contains(t, p.Publish, "waypoint.*.module.drill.drill.state")
	assert.Contains(t, p.Publish, "waypoint.*.module.drill.sensor.state")
	assert.Contains(t, p.Subscribe, "waypoint.*.module.drill.drill.cmd")
}
