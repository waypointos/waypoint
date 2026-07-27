#include <gtest/gtest.h>

#include <map>
#include <optional>
#include <utility>
#include <vector>

#include "control/DriveController.hpp"
#include "joint/JointDriver.hpp"

using namespace wp::control;

namespace {

constexpr Kinematics kKin{.wheelRadius_m = 0.07425, .trackWidth_m = 0.30};
constexpr WheelJoints kWheels{.frontLeft = 10, .frontRight = 9, .backLeft = 7, .backRight = 8};

struct FakeJointDriver : wp::joint::JointDriver {
    std::vector<std::pair<wp::joint::JointId, wp::joint::JointCommand>> written;
    std::map<wp::joint::JointId, wp::joint::JointState> states;
    void writeCommands(
        const std::vector<std::pair<wp::joint::JointId, wp::joint::JointCommand>>& cmds) override {
        for (const auto& c : cmds) written.push_back(c);
    }
    std::optional<wp::joint::JointState> readState(wp::joint::JointId id) override {
        auto it = states.find(id);
        if (it == states.end()) return std::nullopt;
        return it->second;
    }
};

}  // namespace

TEST(DriveController, DisarmedTickDoesNotWrite) {
    // Drive servos in wheel mode treat GOAL_SPEED as a direct motor command
    // independent of TORQUE_ENABLE, so a disarmed controller must emit no
    // writes at all.
    FakeJointDriver drv;
    DriveController dc(&drv, kKin, kWheels);

    dc.setBodyTarget({.vx = 1.0, .omegaZ = 0.5});
    for (int i = 0; i < 25; ++i) dc.tick(0.02);
    EXPECT_TRUE(drv.written.empty());

    // Sanity: arming and ticking with a non-zero target produces writes.
    dc.setArmed(true);
    dc.tick(0.02);
    EXPECT_FALSE(drv.written.empty());
}

TEST(DriveController, ArmedTickPassesTargetThrough) {
    // No outer PID: the seam carries the kinematic target exactly, every tick,
    // regardless of measured speed.
    FakeJointDriver drv;
    DriveController dc(&drv, kKin, kWheels);

    BodyVel target{.vx = 0.5, .omegaZ = 0.1};
    WheelVel desired = kKin.bodyToWheels(target);

    dc.setArmed(true);
    dc.setBodyTarget(target);
    dc.setMeasured(WheelVel{1.0, -1.0, 1.0, -1.0});  // arbitrary noise
    dc.tick(0.02);

    auto findLast = [&](wp::joint::JointId id) -> double {
        double v = 0;
        for (auto& [wid, cmd] : drv.written)
            if (wid == id && cmd.velocityRadps) v = *cmd.velocityRadps;
        return v;
    };
    EXPECT_NEAR(findLast(kWheels.frontLeft),  desired.frontLeft,  1e-9);
    EXPECT_NEAR(findLast(kWheels.frontRight), desired.frontRight, 1e-9);
    EXPECT_NEAR(findLast(kWheels.backLeft),   desired.backLeft,   1e-9);
    EXPECT_NEAR(findLast(kWheels.backRight),  desired.backRight,  1e-9);
}

TEST(DriveController, WritesRoverFrameWheelVelocities) {
    FakeJointDriver drv;
    DriveController dc(&drv,
        wp::control::Kinematics{.wheelRadius_m = 0.1, .trackWidth_m = 0.4},
        wp::control::WheelJoints{.frontLeft = 10, .frontRight = 9, .backLeft = 7, .backRight = 8});
    dc.setArmed(true);
    dc.setBodyTarget({.vx = 1.0, .omegaZ = 0.0});
    dc.tick(0.02);
    ASSERT_EQ(drv.written.size(), 4u);
    for (const auto& [id, cmd] : drv.written) {
        ASSERT_TRUE(cmd.velocityRadps.has_value());
        EXPECT_NEAR(*cmd.velocityRadps, 10.0, 1e-9) << "joint " << int(id);
    }
}

TEST(DriveController, ArmedZeroTargetWritesZero) {
    // Regression: with a previous outer PID, noisy measured speed at a zero
    // setpoint produced reactive writes and a limit cycle. Direct passthrough
    // must write 0 to every wheel regardless of measured noise.
    FakeJointDriver drv;
    DriveController dc(&drv, kKin, kWheels);

    dc.setArmed(true);
    dc.setBodyTarget({});
    dc.setMeasured(WheelVel{0.3, -0.2, 0.4, -0.1});
    for (int i = 0; i < 10; ++i) dc.tick(0.02);

    for (auto& [id, cmd] : drv.written) {
        ASSERT_TRUE(cmd.velocityRadps.has_value());
        EXPECT_DOUBLE_EQ(*cmd.velocityRadps, 0.0);
    }
    EXPECT_FALSE(drv.written.empty());  // it ran, it just wrote zeros
}

TEST(DriveController, ReadsMeasuredFromDriverForTelemetry) {
    // tick() reads measured wheel speeds for telemetry even when disarmed; the
    // seam is rover-frame SI, so DriveController applies no inversion.
    FakeJointDriver drv;
    drv.states[kWheels.frontLeft]  = {.velocityRadps = 1.0};
    drv.states[kWheels.frontRight] = {.velocityRadps = 2.0};
    drv.states[kWheels.backLeft]   = {.velocityRadps = 3.0};
    drv.states[kWheels.backRight]  = {.velocityRadps = 4.0};
    DriveController dc(&drv, kKin, kWheels);

    dc.tick(0.02);
    auto m = dc.lastMeasured();
    EXPECT_DOUBLE_EQ(m.frontLeft,  1.0);
    EXPECT_DOUBLE_EQ(m.frontRight, 2.0);
    EXPECT_DOUBLE_EQ(m.backLeft,   3.0);
    EXPECT_DOUBLE_EQ(m.backRight,  4.0);
    EXPECT_TRUE(drv.written.empty());  // disarmed: no writes
}

TEST(DriveController, DisarmClearsTarget) {
    DriveController dc(nullptr, kKin, kWheels);
    dc.setArmed(true);
    dc.setBodyTarget({.vx = 0.5, .omegaZ = 0.2});
    dc.setArmed(false);
    BodyVel t = dc.target();
    EXPECT_DOUBLE_EQ(t.vx, 0.0);
    EXPECT_DOUBLE_EQ(t.omegaZ, 0.0);
}
