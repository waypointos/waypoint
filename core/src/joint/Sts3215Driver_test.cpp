#include "joint/Sts3215Driver.hpp"

#include <memory>

#include <gtest/gtest.h>

#include "platform/Descriptor.hpp"
#include "servo/MockUart.hpp"
#include "servo/Sts3215Bus.hpp"
#include "servo/Sts3215Frame.hpp"

namespace {

// Two-joint descriptor: one inverted wheel (bus 10), one plain wheel (bus 9).
constexpr char kTwoJoints[] = R"(
schema = 1
[platform]
id = "t"
vehicle_class = "diff_drive_rover"
[drivers.main]
kind = "sim"
[[joints]]
name = "left"
driver = "main"
bus_id = 10
type = "wheel"
ownership = "platform"
invert = true
command_interfaces = ["velocity"]
state_interfaces = ["velocity"]
[joints.limits]
velocity_radps = 10.0
[[joints]]
name = "right"
driver = "main"
bus_id = 9
type = "wheel"
ownership = "platform"
command_interfaces = ["velocity"]
state_interfaces = ["velocity"]
[joints.limits]
velocity_radps = 10.0
[[joints]]
name = "arm"
driver = "main"
bus_id = 1
type = "revolute"
ownership = "module"
command_interfaces = ["position"]
state_interfaces = ["position"]
[kinematics]
model = "diff_drive"
wheel_radius_m = 0.1
track_width_m = 0.3
wheels = { front_left = "left", front_right = "right", back_left = "left", back_right = "right" }
)";
// Note: kinematics.wheels references are irrelevant to the driver under test;
// they exist to satisfy descriptor validation.

struct Capture {
    std::vector<std::vector<std::uint8_t>> frames;
};

// Build a driver over a MockUart that records every written frame and answers
// with a bare status frame (the Sts3215Bus_test.cpp pattern).
std::unique_ptr<wp::servo::Sts3215Bus> makeBus(Capture* cap) {
    auto uart = std::make_unique<wp::servo::MockUart>(
        [cap](const std::vector<std::uint8_t>& req) {
            cap->frames.push_back(req);
            std::uint8_t id = req[2];
            std::uint8_t cs = wp::servo::checksum(id, 0x02, 0x00, nullptr, 0);
            return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
        });
    auto bus = std::make_unique<wp::servo::Sts3215Bus>(std::move(uart));
    bus->open("mock");
    return bus;
}

wp::platform::Descriptor parseDesc() {
    std::string err;
    auto d = wp::platform::Descriptor::parse(kTwoJoints, &err);
    EXPECT_TRUE(d.has_value()) << err;
    return *d;
}

TEST(Sts3215Driver, ResolvesNamesToBusIds) {
    Capture cap;
    auto bus = makeBus(&cap);
    auto d = parseDesc();
    wp::joint::Sts3215Driver drv(bus.get(), d, "main");
    ASSERT_TRUE(drv.idForName("left").has_value());
    EXPECT_EQ(*drv.idForName("left"), 10);
    EXPECT_EQ(*drv.idForName("right"), 9);
    EXPECT_FALSE(drv.idForName("ghost").has_value());
}

// Inversion check without depending on the raw sign-magnitude encoding:
// +1 rad/s on the inverted joint must produce byte-identical GOAL_SPEED
// payload to -1 rad/s on a non-inverted joint (modulo the servo id byte).
TEST(Sts3215Driver, InvertsVelocityCommands) {
    Capture capA;
    auto busA = makeBus(&capA);
    auto d = parseDesc();
    wp::joint::Sts3215Driver drvA(busA.get(), d, "main");
    drvA.writeCommands({{10, {.velocityRadps = 1.0}}});  // inverted joint

    Capture capB;
    auto busB = makeBus(&capB);
    wp::joint::Sts3215Driver drvB(busB.get(), d, "main");
    drvB.writeCommands({{9, {.velocityRadps = -1.0}}});  // plain joint

    ASSERT_EQ(capA.frames.size(), 1u);
    ASSERT_EQ(capB.frames.size(), 1u);
    auto a = capA.frames[0];
    auto b = capB.frames[0];
    ASSERT_EQ(a.size(), b.size());
    // Same instruction, register, and speed payload; ids differ; checksum differs.
    for (size_t i = 3; i + 1 < a.size(); ++i) {
        EXPECT_EQ(a[i], b[i]) << "byte " << i;
    }
}

TEST(Sts3215Driver, PositionCommandWritesGoalPosition) {
    Capture cap;
    auto bus = makeBus(&cap);
    auto d = parseDesc();
    wp::joint::Sts3215Driver drv(bus.get(), d, "main");
    // pi radians = 2048 ticks
    drv.writeCommands({{1, {.positionRad = 3.14159265358979}}});
    ASSERT_EQ(cap.frames.size(), 1u);
    // Frame: FF FF id len instr addr lo hi cs; addr 0x2A = GOAL_POSITION.
    EXPECT_EQ(cap.frames[0][2], 1);
    EXPECT_EQ(cap.frames[0][5], 0x2A);
    std::uint16_t raw = cap.frames[0][6] | (cap.frames[0][7] << 8);
    EXPECT_EQ(raw, 2048);
}
}  // namespace
