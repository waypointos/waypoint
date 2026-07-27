#pragma once

#include <cstdint>
#include <map>
#include <optional>
#include <string>
#include <vector>

namespace wp::platform {

struct Limits {
    std::optional<double> velocityRadps;
    std::optional<double> positionMinRad;
    std::optional<double> positionMaxRad;
};

struct Joint {
    std::string name;
    std::string driver;
    std::uint8_t busId = 0;
    std::string type;       // wheel | revolute | gripper
    std::string ownership;  // platform | module
    bool invert = false;
    std::vector<std::string> commandInterfaces;
    Limits limits;
};

struct DriverCfg {
    std::string kind;  // sts3215 | sim
    std::string port;
    int baud = 1000000;
};

struct DescriptorKinematics {
    std::string model;
    double wheelRadiusM = 0.0;
    double trackWidthM = 0.0;
    std::map<std::string, std::string> wheels;  // position -> joint name
};

// The subset of the platform descriptor core consumes. Authoring-time
// strictness (unknown keys, full rulebook) is the Go validator's job; core
// checks only what it reads and fails closed on structural problems.
struct Descriptor {
    int schema = 0;
    std::string platformId;
    std::string platformName;
    std::string vehicleClass;
    std::map<std::string, DriverCfg> drivers;
    std::vector<Joint> joints;
    std::optional<DescriptorKinematics> kinematics;

    // Parse + structural validation. nullopt on failure with *err set.
    static std::optional<Descriptor> parse(const std::string& toml, std::string* err);
    static std::optional<Descriptor> load(const std::string& path, std::string* err);

    // Drive is a capability: present iff the vehicle class declares
    // diff-drive kinematics.
    bool hasDrive() const { return kinematics.has_value(); }

    const Joint* jointByName(const std::string& name) const;
    std::vector<std::uint8_t> platformOwnedBusIds() const;
    // Module-owned continuous-rotation servos, the set an estop must zero.
    std::vector<std::uint8_t> moduleWheelBusIds() const;
    std::vector<std::uint8_t> allBusIds() const;
};

}  // namespace wp::platform
