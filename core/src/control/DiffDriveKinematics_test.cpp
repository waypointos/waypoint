#include <gtest/gtest.h>

#include "control/DiffDriveKinematics.hpp"

using namespace wp::control;

TEST(DiffDrive, StraightAhead) {
    Kinematics k{.wheelRadius_m = 0.07425, .trackWidth_m = 0.30};
    auto out = k.bodyToWheels({.vx = 1.0, .omegaZ = 0.0});
    EXPECT_NEAR(out.frontLeft,  1.0 / 0.07425, 1e-9);
    EXPECT_NEAR(out.frontRight, 1.0 / 0.07425, 1e-9);
    EXPECT_NEAR(out.backLeft,   1.0 / 0.07425, 1e-9);
    EXPECT_NEAR(out.backRight,  1.0 / 0.07425, 1e-9);
}

TEST(DiffDrive, PureRotation) {
    Kinematics k{.wheelRadius_m = 0.07425, .trackWidth_m = 0.30};
    auto out = k.bodyToWheels({.vx = 0.0, .omegaZ = 1.0});
    // Left side spins backward, right side forward.
    EXPECT_LT(out.frontLeft, 0);
    EXPECT_GT(out.frontRight, 0);
    EXPECT_NEAR(out.frontLeft, -out.frontRight, 1e-9);
}
