#pragma once

#include <map>
#include <optional>
#include <string>
#include <vector>

#include "joint/JointDriver.hpp"
#include "platform/Descriptor.hpp"
#include "servo/Sts3215Bus.hpp"

namespace wp::joint {

// Real STS3215 backend for the joint seam. Owns name-to-bus-id resolution,
// tick/SI conversion, velocity inversion (mount mirroring affects rotation
// direction; absolute encoder position is reported unmirrored), and the
// EEPROM operating-mode bring-up driven by descriptor joint types.
class Sts3215Driver : public JointDriver {
public:
    Sts3215Driver(servo::Sts3215Bus* bus, const platform::Descriptor& d,
                  const std::string& driverName);

    std::optional<JointId> idForName(const std::string& name) const;

    // EEPROM mode-ensure for the given present ids: wheel mode for wheel-type
    // joints, position mode otherwise. Returns how many succeeded.
    int ensureModes(const std::vector<JointId>& presentIds);

    int setTorqueEnable(JointId id, bool on);

    void writeCommands(const std::vector<std::pair<JointId, JointCommand>>& cmds) override;
    std::optional<JointState> readState(JointId id) override;

private:
    struct Info {
        bool invert = false;
        bool isWheel = false;
    };
    servo::Sts3215Bus* bus_;
    std::map<std::string, JointId> byName_;
    std::map<JointId, Info> info_;
};

}  // namespace wp::joint
