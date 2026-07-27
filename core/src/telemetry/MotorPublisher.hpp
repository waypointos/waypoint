#pragma once

#include <cstdint>
#include <string>

#include "nats/Connection.hpp"
#include "servo/Sts3215Bus.hpp"

namespace wp::telemetry {

// MotorPublisher emits one MotorTelemetry per servo per tick (Phase 1 spec).
// Caller is expected to invoke publish() at ~20 Hz with current readings.
class MotorPublisher {
public:
    MotorPublisher(wp::nats::Connection* nc, const std::string& roverID)
        : nc_(nc), subject_("waypoint." + roverID + ".telemetry.motors") {}

    // Publishes a single servo's telemetry frame.
    void publish(std::uint8_t id, const servo::ServoState& s);

private:
    wp::nats::Connection* nc_;
    std::string subject_;
};

}  // namespace wp::telemetry
