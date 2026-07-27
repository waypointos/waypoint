#pragma once

#include <cstddef>
#include <cstdint>
#include <set>
#include <unordered_map>
#include <vector>

namespace wp::servo {

// Round-robin selector over the non-drive servos so telemetry reads of them
// stay bounded per control tick. A servo that fails `maxFailures` reads in a
// row is dropped from the rotation, so a loose cable can't reintroduce
// per-tick bus timeouts; re-detection requires a restart.
class AuxRotation {
public:
    AuxRotation(std::vector<std::uint8_t> ids, int maxFailures);

    // Up to `k` distinct still-active ids, advancing the cursor (wraps).
    std::vector<std::uint8_t> next(int k);

    void recordResult(std::uint8_t id, bool ok);

    std::vector<std::uint8_t> active() const;

private:
    std::vector<std::uint8_t> ids_;
    std::set<std::uint8_t> dropped_;
    std::unordered_map<std::uint8_t, int> fails_;
    int maxFailures_;
    std::size_t cursor_ = 0;
};

}  // namespace wp::servo
