#include "telemetry/PlatformPublisher.hpp"

#include <gtest/gtest.h>

#include "platform/Descriptor.hpp"

namespace {
const char* kBench = R"(
schema = 1
[platform]
id = "waypoint-bench"
name = "Waypoint Bench"
vehicle_class = "fixed_base"
[drivers.main]
kind = "sts3215"
port = "/dev/ttyAMA0"
[[joints]]
name = "arm_1"
driver = "main"
bus_id = 1
type = "revolute"
ownership = "module"
command_interfaces = ["position"]
)";

const char* kMiniRover = R"(
schema = 1
[platform]
id = "mini"
vehicle_class = "diff_drive_rover"
[drivers.main]
kind = "sts3215"
port = "/dev/ttyAMA0"
[[joints]]
name = "wl_f"
driver = "main"
bus_id = 1
type = "wheel"
ownership = "platform"
invert = true
[[joints]]
name = "wr_f"
driver = "main"
bus_id = 2
type = "wheel"
ownership = "platform"
[[joints]]
name = "wl_b"
driver = "main"
bus_id = 3
type = "wheel"
ownership = "platform"
invert = true
[[joints]]
name = "wr_b"
driver = "main"
bus_id = 4
type = "wheel"
ownership = "platform"
[kinematics]
model = "diff_drive"
wheel_radius_m = 0.05
track_width_m = 0.2
wheels = { front_left = "wl_f", front_right = "wr_f", back_left = "wl_b", back_right = "wr_b" }
)";
}  // namespace

TEST(PlatformInfoProjection, FixedBaseHasNoKinematics) {
    std::string err;
    auto d = wp::platform::Descriptor::parse(kBench, &err);
    ASSERT_TRUE(d) << err;
    auto info = wp::telemetry::buildPlatformInfo(*d);
    EXPECT_EQ(info.platform_id(), "waypoint-bench");
    EXPECT_EQ(info.name(), "Waypoint Bench");
    EXPECT_EQ(info.vehicle_class(), "fixed_base");
    EXPECT_EQ(info.schema(), 1u);
    ASSERT_EQ(info.joints_size(), 1);
    EXPECT_EQ(info.joints(0).name(), "arm_1");
    EXPECT_EQ(info.joints(0).bus_id(), 1u);
    EXPECT_EQ(info.joints(0).ownership(), "module");
    ASSERT_EQ(info.joints(0).command_interfaces_size(), 1);
    EXPECT_EQ(info.joints(0).command_interfaces(0), "position");
    EXPECT_FALSE(info.has_kinematics());
}

TEST(PlatformInfoProjection, DiffDriveCarriesKinematics) {
    std::string err;
    auto d = wp::platform::Descriptor::parse(kMiniRover, &err);
    ASSERT_TRUE(d) << err;
    auto info = wp::telemetry::buildPlatformInfo(*d);
    ASSERT_TRUE(info.has_kinematics());
    EXPECT_DOUBLE_EQ(info.kinematics().wheel_radius_m(), 0.05);
    EXPECT_DOUBLE_EQ(info.kinematics().track_width_m(), 0.2);
    EXPECT_EQ(info.kinematics().wheels().at("front_left"), "wl_f");
    EXPECT_EQ(info.joints(0).invert(), true);
}
