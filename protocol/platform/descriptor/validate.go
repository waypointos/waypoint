package descriptor

import "fmt"

const supportedSchema = 1

var (
	vehicleClasses  = map[string]bool{"diff_drive_rover": true, "fixed_base": true}
	driverKinds     = map[string]bool{"sts3215": true, "sim": true}
	jointTypes      = map[string]bool{"wheel": true, "revolute": true, "gripper": true}
	ownerships      = map[string]bool{"platform": true, "module": true}
	cmdInterfaces   = map[string]bool{"position": true, "velocity": true, "effort": true}
	stateInterfaces = map[string]bool{
		"position": true, "velocity": true, "load": true,
		"current": true, "voltage": true, "temperature": true,
	}
	sensorKinds    = map[string]bool{"imu": true, "gps": true, "camera": true}
	wheelPositions = []string{"front_left", "front_right", "back_left", "back_right"}
)

func (d *Descriptor) validate() error {
	if d.Schema != supportedSchema {
		return fmt.Errorf("descriptor: unsupported schema %d (supported: %d)", d.Schema, supportedSchema)
	}
	if d.Platform.ID == "" {
		return fmt.Errorf("descriptor: platform.id is required")
	}
	if !vehicleClasses[d.Platform.VehicleClass] {
		return fmt.Errorf("descriptor: vehicle_class %q is not a known class", d.Platform.VehicleClass)
	}
	for name, drv := range d.Drivers {
		if !driverKinds[drv.Kind] {
			return fmt.Errorf("descriptor: driver %q kind %q is not a known kind", name, drv.Kind)
		}
		if drv.Kind == "sts3215" && drv.Port == "" {
			return fmt.Errorf("descriptor: driver %q (sts3215) requires port", name)
		}
	}
	if err := d.validateJoints(); err != nil {
		return err
	}
	if err := d.validateKinematics(); err != nil {
		return err
	}
	for _, s := range d.Sensors {
		if !sensorKinds[s.Kind] {
			return fmt.Errorf("descriptor: sensor %q kind %q is not a known kind", s.Name, s.Kind)
		}
	}
	return nil
}

func (d *Descriptor) validateJoints() error {
	names := map[string]bool{}
	busIDs := map[string]map[uint32]bool{} // per driver
	for _, j := range d.Joints {
		if j.Name == "" {
			return fmt.Errorf("descriptor: joint with empty name")
		}
		if names[j.Name] {
			return fmt.Errorf("descriptor: duplicate joint name %q", j.Name)
		}
		names[j.Name] = true
		if _, ok := d.Drivers[j.Driver]; !ok {
			return fmt.Errorf("descriptor: joint %q references undefined driver %q", j.Name, j.Driver)
		}
		if busIDs[j.Driver] == nil {
			busIDs[j.Driver] = map[uint32]bool{}
		}
		if busIDs[j.Driver][j.BusID] {
			return fmt.Errorf("descriptor: duplicate bus_id %d on driver %q", j.BusID, j.Driver)
		}
		busIDs[j.Driver][j.BusID] = true
		if !jointTypes[j.Type] {
			return fmt.Errorf("descriptor: joint %q has unknown joint type %q", j.Name, j.Type)
		}
		if !ownerships[j.Ownership] {
			return fmt.Errorf("descriptor: joint %q has unknown ownership %q", j.Name, j.Ownership)
		}
		for _, c := range j.CommandInterfaces {
			if !cmdInterfaces[c] {
				return fmt.Errorf("descriptor: joint %q command_interfaces has unknown value %q", j.Name, c)
			}
		}
		for _, s := range j.StateInterfaces {
			if !stateInterfaces[s] {
				return fmt.Errorf("descriptor: joint %q state_interfaces has unknown value %q", j.Name, s)
			}
		}
		// Platform-owned joints must declare the limit the platform enforces.
		// Module-owned joints calibrate their own (so100 measures hard stops).
		if j.Ownership == "platform" {
			switch j.Type {
			case "wheel":
				if j.Limits == nil || j.Limits.VelocityRadps == nil {
					return fmt.Errorf("descriptor: platform-owned wheel joint %q requires limits.velocity_radps", j.Name)
				}
			case "revolute", "gripper":
				if j.Limits == nil || j.Limits.PositionMinRad == nil || j.Limits.PositionMaxRad == nil {
					return fmt.Errorf("descriptor: platform-owned joint %q requires limits.position_min_rad and position_max_rad", j.Name)
				}
			}
		}
	}
	return nil
}

func (d *Descriptor) validateKinematics() error {
	if d.Platform.VehicleClass == "fixed_base" {
		if d.Kinematics != nil {
			return fmt.Errorf("descriptor: vehicle_class fixed_base must not declare [kinematics]")
		}
		return nil
	}
	if d.Platform.VehicleClass != "diff_drive_rover" {
		return nil
	}
	k := d.Kinematics
	if k == nil {
		return fmt.Errorf("descriptor: vehicle_class diff_drive_rover requires [kinematics]")
	}
	if k.Model != "diff_drive" {
		return fmt.Errorf("descriptor: kinematics.model %q does not match vehicle_class diff_drive_rover", k.Model)
	}
	if k.WheelRadiusM <= 0 || k.TrackWidthM <= 0 {
		return fmt.Errorf("descriptor: kinematics wheel_radius_m and track_width_m must be positive")
	}
	byName := map[string]Joint{}
	for _, j := range d.Joints {
		byName[j.Name] = j
	}
	for _, pos := range wheelPositions {
		name, ok := k.Wheels[pos]
		if !ok {
			return fmt.Errorf("descriptor: kinematics.wheels missing position %q", pos)
		}
		j, ok := byName[name]
		if !ok {
			return fmt.Errorf("descriptor: kinematics.wheels %s references unknown joint %q", pos, name)
		}
		if j.Type != "wheel" {
			return fmt.Errorf("descriptor: kinematics.wheels %s references non-wheel joint %q", pos, name)
		}
	}
	for pos := range k.Wheels {
		known := false
		for _, p := range wheelPositions {
			if p == pos {
				known = true
			}
		}
		if !known {
			return fmt.Errorf("descriptor: kinematics.wheels has unknown position %q", pos)
		}
	}
	return nil
}
