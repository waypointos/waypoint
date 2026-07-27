#pragma once

namespace wp::control {

struct BodyVel {
    double vx = 0.0;       // m/s, forward
    double omegaZ = 0.0;   // rad/s, yaw
};

struct WheelVel {
    double frontLeft = 0.0;
    double frontRight = 0.0;
    double backLeft = 0.0;
    double backRight = 0.0;
};

struct Kinematics {
    double wheelRadius_m;
    double trackWidth_m;

    WheelVel bodyToWheels(BodyVel b) const;
};

}  // namespace wp::control
