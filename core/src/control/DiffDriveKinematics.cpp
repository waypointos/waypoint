#include "control/DiffDriveKinematics.hpp"

namespace wp::control {

WheelVel Kinematics::bodyToWheels(BodyVel b) const {
    double vL = b.vx - (b.omegaZ * trackWidth_m / 2.0);
    double vR = b.vx + (b.omegaZ * trackWidth_m / 2.0);
    double wL = vL / wheelRadius_m;
    double wR = vR / wheelRadius_m;
    return {.frontLeft = wL, .frontRight = wR, .backLeft = wL, .backRight = wR};
}

}  // namespace wp::control
