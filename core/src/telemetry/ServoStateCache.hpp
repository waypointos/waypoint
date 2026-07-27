#pragma once

#include <chrono>
#include <cstdint>
#include <map>
#include <optional>

#include "servo/Sts3215Bus.hpp"  // ServoState

namespace wp::telemetry {

// Last-known per-servo readings, written by the telemetry read loop and
// aggregated for power telemetry. The caller supplies the time so the loop's
// steady_clock drives freshness and tests can inject timestamps.
class ServoStateCache {
public:
    using Clock = std::chrono::steady_clock;

    void update(std::uint8_t id, const wp::servo::ServoState& s, Clock::time_point now);

    // Total current draw (amps) across servos last read within `window`.
    double sumCurrentFresh(Clock::time_point now, Clock::duration window) const;

    // Bus voltage (volts) from the most recently read servo within `window`;
    // nullopt if none are fresh. Servos on one bus agree to within a few mV.
    std::optional<double> freshestVoltage(Clock::time_point now, Clock::duration window) const;

private:
    struct Entry {
        wp::servo::ServoState state;
        Clock::time_point at;
    };
    std::map<std::uint8_t, Entry> entries_;
};

}  // namespace wp::telemetry
