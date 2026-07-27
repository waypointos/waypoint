#pragma once

#include <algorithm>
#include <array>
#include <cstdint>
#include <functional>
#include <mutex>
#include <string>
#include <vector>

#include "messages/servo.pb.h"
#include "nats/Connection.hpp"
#include "servo/Sts3215Bus.hpp"

namespace wp::servo {

// Generic per-servo control surface. Subscribes cmd.servo and answers
// rpc.servo_read. Arm-agnostic: it knows servos, not arms or modules.
class ServoControlServer {
public:
    ServoControlServer(wp::nats::Connection* nc, Sts3215Bus* bus,
                       std::function<bool()> isEstopped, const std::string& roverID,
                       std::vector<std::uint8_t> denyIds = {});

    // Entry points (NATS callbacks delegate here; also the unit-test surface).
    void handleControl(const waypoint::v1::ServoControl& c);
    void handleSync(const waypoint::v1::ServoSyncWrite& s);
    waypoint::v1::ServoState handleRead(std::uint32_t servoID);

private:
    bool isDenied(std::uint32_t id) const;

    wp::nats::Connection* nc_;
    Sts3215Bus* bus_;
    std::function<bool()> isEstopped_;
    std::vector<std::uint8_t> deny_;

    std::mutex ceilMu_;
    std::array<std::uint16_t, 13> overcurrentCeil_{};  // index by id 1..12; 0 = trip disabled
};

}  // namespace wp::servo
