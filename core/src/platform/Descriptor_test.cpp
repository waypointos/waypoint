#include "platform/Descriptor.hpp"

#include <algorithm>

#include <gtest/gtest.h>

namespace {

constexpr char kMinimal[] = R"(
schema = 1

[platform]
id = "test-rover"
vehicle_class = "diff_drive_rover"

[drivers.main]
kind = "sim"

[[joints]]
name = "wheel_front_left"
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
name = "wheel_front_right"
driver = "main"
bus_id = 9
type = "wheel"
ownership = "platform"
command_interfaces = ["velocity"]
state_interfaces = ["velocity"]
[joints.limits]
velocity_radps = 10.0

[[joints]]
name = "wheel_back_left"
driver = "main"
bus_id = 7
type = "wheel"
ownership = "platform"
invert = true
command_interfaces = ["velocity"]
state_interfaces = ["velocity"]
[joints.limits]
velocity_radps = 10.0

[[joints]]
name = "wheel_back_right"
driver = "main"
bus_id = 8
type = "wheel"
ownership = "platform"
command_interfaces = ["velocity"]
state_interfaces = ["velocity"]
[joints.limits]
velocity_radps = 10.0

[[joints]]
name = "arm_1"
driver = "main"
bus_id = 1
type = "revolute"
ownership = "module"
command_interfaces = ["position"]
state_interfaces = ["position"]

[kinematics]
model = "diff_drive"
wheel_radius_m = 0.07425
track_width_m = 0.30
wheels = { front_left = "wheel_front_left", front_right = "wheel_front_right", back_left = "wheel_back_left", back_right = "wheel_back_right" }
)";

TEST(Descriptor, ParsesMinimal) {
    std::string err;
    auto d = wp::platform::Descriptor::parse(kMinimal, &err);
    ASSERT_TRUE(d.has_value()) << err;
    EXPECT_EQ(d->schema, 1);
    EXPECT_EQ(d->platformId, "test-rover");
    EXPECT_EQ(d->joints.size(), 5u);
    EXPECT_EQ(d->drivers.at("main").kind, "sim");
    ASSERT_TRUE(d->hasDrive());
    EXPECT_DOUBLE_EQ(d->kinematics->trackWidthM, 0.30);
    auto* fl = d->jointByName("wheel_front_left");
    ASSERT_NE(fl, nullptr);
    EXPECT_EQ(fl->busId, 10);
    EXPECT_TRUE(fl->invert);
}

TEST(Descriptor, RejectsWrongSchema) {
    std::string toml(kMinimal);
    toml.replace(toml.find("schema = 1"), 10, "schema = 2");
    std::string err;
    EXPECT_FALSE(wp::platform::Descriptor::parse(toml, &err).has_value());
    EXPECT_NE(err.find("schema"), std::string::npos);
}

TEST(Descriptor, RejectsUnknownDriverRef) {
    std::string toml(kMinimal);
    toml.replace(toml.find("driver = \"main\""), 15, "driver = \"ghst\"");
    std::string err;
    EXPECT_FALSE(wp::platform::Descriptor::parse(toml, &err).has_value());
    EXPECT_NE(err.find("driver"), std::string::npos);
}

TEST(Descriptor, RejectsDuplicateBusID) {
    std::string toml(kMinimal);
    toml.replace(toml.find("bus_id = 9"), 10, "bus_id = 10");
    std::string err;
    EXPECT_FALSE(wp::platform::Descriptor::parse(toml, &err).has_value());
    EXPECT_NE(err.find("bus_id"), std::string::npos);
}

TEST(Descriptor, RejectsWheelsRefUnknownJoint) {
    std::string toml(kMinimal);
    toml.replace(toml.find("front_left = \"wheel_front_left\""), 31,
                 "front_left = \"ghost_wheel_axle\"");
    std::string err;
    EXPECT_FALSE(wp::platform::Descriptor::parse(toml, &err).has_value());
    EXPECT_NE(err.find("wheels"), std::string::npos);
}

TEST(Descriptor, DerivedSets) {
    std::string err;
    auto d = wp::platform::Descriptor::parse(kMinimal, &err);
    ASSERT_TRUE(d.has_value()) << err;
    auto deny = d->platformOwnedBusIds();
    std::sort(deny.begin(), deny.end());
    EXPECT_EQ(deny, (std::vector<std::uint8_t>{7, 8, 9, 10}));
    auto all = d->allBusIds();
    EXPECT_EQ(all.size(), 5u);
}

TEST(Descriptor, FixedBaseParsesWithoutKinematics) {
    std::string err;
    auto d = wp::platform::Descriptor::parse(R"(
schema = 1
[platform]
id = "bench"
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
)", &err);
    ASSERT_TRUE(d) << err;
    EXPECT_FALSE(d->hasDrive());
    EXPECT_EQ(d->vehicleClass, "fixed_base");
}

TEST(Descriptor, FixedBaseRejectsKinematics) {
    std::string err;
    auto d = wp::platform::Descriptor::parse(R"(
schema = 1
[platform]
id = "bench"
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
[kinematics]
model = "diff_drive"
wheel_radius_m = 0.07
track_width_m = 0.3
)", &err);
    EXPECT_FALSE(d);
    EXPECT_NE(err.find("fixed_base must not declare"), std::string::npos) << err;
}

TEST(Descriptor, UnknownVehicleClassRejected) {
    std::string err;
    auto d = wp::platform::Descriptor::parse(R"(
schema = 1
[platform]
id = "x"
vehicle_class = "hovercraft"
[drivers.main]
kind = "sim"
[[joints]]
name = "j"
driver = "main"
bus_id = 1
type = "revolute"
ownership = "module"
)", &err);
    EXPECT_FALSE(d);
    EXPECT_NE(err.find("unknown vehicle_class"), std::string::npos) << err;
}

TEST(Descriptor, CommandInterfacesParsed) {
    std::string err;
    auto d = wp::platform::Descriptor::parse(R"(
schema = 1
[platform]
id = "x"
vehicle_class = "fixed_base"
[drivers.main]
kind = "sim"
[[joints]]
name = "j1"
driver = "main"
bus_id = 1
type = "wheel"
ownership = "module"
command_interfaces = ["velocity", "position"]
)", &err);
    ASSERT_TRUE(d) << err;
    EXPECT_EQ(d->joints[0].commandInterfaces,
              (std::vector<std::string>{"velocity", "position"}));
}
}  // namespace

#include <fstream>

#include <nlohmann/json.hpp>

namespace {

// Mirrors the Go/TS derivation: rover-frame wheel omegas, then per-joint
// inversion. The golden values are what a driver writes. Drive-only: callers
// guard on hasDrive() since a fixed_base platform carries no kinematics.
std::map<std::string, double> goldenWheelSpeeds(const wp::platform::Descriptor& d,
                                                double vx, double wz) {
    const auto& k = *d.kinematics;
    double vL = vx - wz * k.trackWidthM / 2.0;
    double vR = vx + wz * k.trackWidthM / 2.0;
    std::map<std::string, double> out;
    for (const auto& [pos, name] : k.wheels) {
        double v = (pos == "front_right" || pos == "back_right") ? vR : vL;
        double w = v / k.wheelRadiusM;
        const auto* j = d.jointByName(name);
        if (j->invert) w = -w;
        out[name] = w;
    }
    return out;
}

void checkGoldenFixture(const std::string& toml, const std::string& golden) {
    const std::string dir = WP_PLATFORM_DIR;
    std::string err;
    auto d = wp::platform::Descriptor::load(dir + "/" + toml, &err);
    ASSERT_TRUE(d.has_value()) << err;

    std::ifstream f(dir + "/" + golden);
    ASSERT_TRUE(f.good()) << "golden fixture missing: " << golden;
    nlohmann::json g = nlohmann::json::parse(f);

    for (auto& [name, id] : g["joint_name_to_bus_id"].items()) {
        const auto* j = d->jointByName(name);
        ASSERT_NE(j, nullptr) << name;
        EXPECT_EQ(j->busId, id.get<int>()) << name;
    }
    EXPECT_EQ(d->joints.size(), g["joint_name_to_bus_id"].size());

    auto deny = d->platformOwnedBusIds();
    std::sort(deny.begin(), deny.end());
    std::vector<std::uint8_t> wantDeny;
    for (auto& v : g["platform_owned_bus_ids"]) wantDeny.push_back(v.get<int>());
    EXPECT_EQ(deny, wantDeny);

    for (auto& c : g["wheel_speeds"]) {
        auto got = goldenWheelSpeeds(*d, c["vx_mps"].get<double>(), c["wz_radps"].get<double>());
        for (auto& [name, want] : c["expected"].items()) {
            ASSERT_TRUE(got.count(name)) << name;
            EXPECT_NEAR(got[name], want.get<double>(), 1e-9) << name;
        }
    }
}

TEST(DescriptorConformance, MatchesGoldenFixtures) {
    checkGoldenFixture("waypoint-rover.toml", "testdata/waypoint-rover.derived.golden.json");
    checkGoldenFixture("waypoint-bench.toml", "testdata/waypoint-bench.derived.golden.json");
}

TEST(DescriptorConformance, ModuleWheelBusIdsFromFixtures) {
    const std::string dir = WP_PLATFORM_DIR;
    std::string err;

    auto rover = wp::platform::Descriptor::load(dir + "/waypoint-rover.toml", &err);
    ASSERT_TRUE(rover.has_value()) << err;
    auto ids = rover->moduleWheelBusIds();
    std::sort(ids.begin(), ids.end());
    EXPECT_EQ(ids, (std::vector<std::uint8_t>{11, 12}));  // drill_1, drill_2

    auto bench = wp::platform::Descriptor::load(dir + "/waypoint-bench.toml", &err);
    ASSERT_TRUE(bench.has_value()) << err;
    EXPECT_TRUE(bench->moduleWheelBusIds().empty());
}
}  // namespace
