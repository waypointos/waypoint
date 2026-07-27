#pragma once

#include <cstdint>
#include <optional>
#include <utility>
#include <vector>

namespace wp::joint {

// Driver-internal joint handle, resolved once from a descriptor joint name.
using JointId = std::uint8_t;

// SI, rover-frame. The seam never carries ticks or mount-frame signs.
// Members default to nullopt so a designated initializer may set only the
// field it commands without tripping -Wmissing-field-initializers.
struct JointCommand {
    std::optional<double> velocityRadps{};
    std::optional<double> positionRad{};
    std::optional<double> effort{};
};

// nullopt is N/A, per the project telemetry rule.
struct JointState {
    std::optional<double> positionRad{};
    std::optional<double> velocityRadps{};
    std::optional<double> load{};
    std::optional<double> currentA{};
    std::optional<double> voltageV{};
    std::optional<double> temperatureC{};
};

// The actuator seam. Real backend: Sts3215Driver; the sim driver implements
// this same interface.
class JointDriver {
public:
    virtual ~JointDriver() = default;
    virtual void writeCommands(const std::vector<std::pair<JointId, JointCommand>>& cmds) = 0;
    virtual std::optional<JointState> readState(JointId id) = 0;
};

}  // namespace wp::joint
