package descriptor

import "fmt"

// BusIDFor resolves a joint name to its bus id.
func (d *Descriptor) BusIDFor(name string) (uint32, bool) {
	for _, j := range d.Joints {
		if j.Name == name {
			return j.BusID, true
		}
	}
	return 0, false
}

// NameForBusID resolves a (driver, bus id) pair to a joint name.
func (d *Descriptor) NameForBusID(driver string, id uint32) (string, bool) {
	for _, j := range d.Joints {
		if j.Driver == driver && j.BusID == id {
			return j.Name, true
		}
	}
	return "", false
}

// PlatformOwnedBusIDs is the deny-list for the module-facing servo surface:
// bus ids of joints a module must never command.
func (d *Descriptor) PlatformOwnedBusIDs() []uint32 {
	var out []uint32
	for _, j := range d.Joints {
		if j.Ownership == "platform" {
			out = append(out, j.BusID)
		}
	}
	return out
}

// WheelSpeeds maps a body twist to signed wheel angular velocities (rad/s),
// keyed by joint name, with per-joint inversion applied. The result is what a
// driver writes, not the rover-frame value.
func (d *Descriptor) WheelSpeeds(vxMps, wzRadps float64) (map[string]float64, error) {
	k := d.Kinematics
	if k == nil || k.Model != "diff_drive" {
		return nil, fmt.Errorf("descriptor: WheelSpeeds requires diff_drive kinematics")
	}
	byName := map[string]Joint{}
	for _, j := range d.Joints {
		byName[j.Name] = j
	}
	vL := vxMps - wzRadps*k.TrackWidthM/2
	vR := vxMps + wzRadps*k.TrackWidthM/2
	out := map[string]float64{}
	for pos, name := range k.Wheels {
		v := vL
		if pos == "front_right" || pos == "back_right" {
			v = vR
		}
		w := v / k.WheelRadiusM
		if byName[name].Invert {
			w = -w
		}
		out[name] = w
	}
	return out, nil
}
