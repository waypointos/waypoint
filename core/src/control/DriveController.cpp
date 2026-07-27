#include "control/DriveController.hpp"

namespace wp::control {

DriveController::DriveController(joint::JointDriver* driver, Kinematics k, WheelJoints joints)
    : driver_(driver), k_(k), joints_(joints) {}

void DriveController::setBodyTarget(BodyVel t) { target_ = t; }

void DriveController::setArmed(bool armed) {
    armed_ = armed;
    if (!armed) target_ = BodyVel{};
}

void DriveController::tick(double /*dt*/) {
    // Always read measured for telemetry, even when disarmed. The seam is
    // rover-frame SI; mount inversion lives in the driver.
    if (!useMeasuredOverride_ && driver_) {
        WheelVel m{};
        if (auto s = driver_->readState(joints_.frontLeft); s && s->velocityRadps)
            m.frontLeft = *s->velocityRadps;
        if (auto s = driver_->readState(joints_.frontRight); s && s->velocityRadps)
            m.frontRight = *s->velocityRadps;
        if (auto s = driver_->readState(joints_.backLeft); s && s->velocityRadps)
            m.backLeft = *s->velocityRadps;
        if (auto s = driver_->readState(joints_.backRight); s && s->velocityRadps)
            m.backRight = *s->velocityRadps;
        measured_ = m;
    }

    if (!armed_ || !driver_) return;

    WheelVel desired = k_.bodyToWheels(target_);
    driver_->writeCommands({
        {joints_.frontLeft,  {.velocityRadps = desired.frontLeft}},
        {joints_.frontRight, {.velocityRadps = desired.frontRight}},
        {joints_.backLeft,   {.velocityRadps = desired.backLeft}},
        {joints_.backRight,  {.velocityRadps = desired.backRight}},
    });
}

}  // namespace wp::control
