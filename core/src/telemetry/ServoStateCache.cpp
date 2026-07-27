#include "telemetry/ServoStateCache.hpp"

namespace wp::telemetry {

namespace {
constexpr double kAmpsPerCurrentUnit = 0.0065;  // STS3215 datasheet: 6.5 mA / unit
constexpr double kVoltsPerDeci = 0.1;
}  // namespace

void ServoStateCache::update(std::uint8_t id, const wp::servo::ServoState& s, Clock::time_point now) {
    entries_[id] = Entry{s, now};
}

double ServoStateCache::sumCurrentFresh(Clock::time_point now, Clock::duration window) const {
    // PRESENT_CURRENT is sign-magnitude: bit 15 = direction, bits 0..14 = magnitude.
    // Power draw cares about magnitude, so mask the direction bit before summing.
    double sum = 0.0;
    for (const auto& kv : entries_) {
        if (now - kv.second.at <= window) {
            std::uint16_t mag = kv.second.state.currentRaw & 0x7FFF;
            sum += static_cast<double>(mag) * kAmpsPerCurrentUnit;
        }
    }
    return sum;
}

std::optional<double> ServoStateCache::freshestVoltage(Clock::time_point now, Clock::duration window) const {
    std::optional<Clock::time_point> best;
    double volts = 0.0;
    for (const auto& kv : entries_) {
        if (now - kv.second.at > window) continue;
        if (!best || kv.second.at > *best) {
            best = kv.second.at;
            volts = static_cast<double>(kv.second.state.voltageDeci) * kVoltsPerDeci;
        }
    }
    if (!best) return std::nullopt;
    return volts;
}

}  // namespace wp::telemetry
