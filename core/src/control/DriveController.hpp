#pragma once

#include "control/DiffDriveKinematics.hpp"
#include "joint/JointDriver.hpp"

namespace wp::control {

// The four drive joints, resolved from the platform descriptor at setup.
struct WheelJoints {
    joint::JointId frontLeft{};
    joint::JointId frontRight{};
    joint::JointId backLeft{};
    joint::JointId backRight{};
};

class DriveController {
public:
    DriveController(joint::JointDriver* driver, Kinematics k, WheelJoints joints);

    // Update target. Threadsafe (atomic snapshot).
    void setBodyTarget(BodyVel t);

    // Tick once at the control rate (default caller cadence: 50 Hz). Reads
    // current wheel speeds for telemetry, writes goal speeds straight through
    // the joint seam. STS3215 in wheel mode is itself a velocity controller,
    // so no outer loop is needed: cascading one caused a limit cycle around
    // a zero setpoint (any PRESENT_SPEED noise produced reactive GOAL_SPEED
    // writes that the servo executed, twitching the wheels).
    void tick(double dt);

    // When disarmed, tick() reads measured speeds for telemetry but emits no
    // writes and clears target. The driver-side torque gate alone is not
    // enough: STS3215 in wheel mode acts on GOAL_SPEED regardless of
    // TORQUE_ENABLE, so stopping the writes is what keeps the wheels still.
    void setArmed(bool armed);
    bool armed() const { return armed_; }

    // For tests: inject measured wheel speeds instead of reading the driver.
    void setMeasured(WheelVel measured) { measured_ = measured; useMeasuredOverride_ = true; }

    BodyVel target() const { return target_; }
    WheelVel lastMeasured() const { return measured_; }
    WheelJoints wheelJoints() const { return joints_; }

private:
    joint::JointDriver* driver_;
    Kinematics k_;
    WheelJoints joints_;
    BodyVel target_{};
    WheelVel measured_{};
    bool useMeasuredOverride_ = false;
    bool armed_ = false;
};

}  // namespace wp::control
