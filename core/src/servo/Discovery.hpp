#pragma once

#include <cstdint>
#include <vector>

#include "servo/Sts3215Bus.hpp"

namespace wp::servo {

// Ping every id in [lo, hi] once and return those that answer, in ascending
// order. Used at boot to learn which servos are on the bus. `pingTimeoutMs`
// bounds the wait for each id so absent ids don't stall the scan.
std::vector<std::uint8_t> discoverServos(Sts3215Bus& bus, std::uint8_t lo, std::uint8_t hi,
                                         int pingTimeoutMs);

// Ping exactly these ids and return those that answer, in input order.
std::vector<std::uint8_t> discoverServos(Sts3215Bus& bus,
                                         const std::vector<std::uint8_t>& ids,
                                         int pingTimeoutMs);

}  // namespace wp::servo
