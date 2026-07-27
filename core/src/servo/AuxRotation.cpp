#include "servo/AuxRotation.hpp"

namespace wp::servo {

AuxRotation::AuxRotation(std::vector<std::uint8_t> ids, int maxFailures)
    : ids_(std::move(ids)), maxFailures_(maxFailures) {}

std::vector<std::uint8_t> AuxRotation::next(int k) {
    std::vector<std::uint8_t> out;
    if (ids_.empty() || k <= 0) return out;
    std::size_t scanned = 0;
    while (static_cast<int>(out.size()) < k && scanned < ids_.size()) {
        std::uint8_t id = ids_[cursor_];
        cursor_ = (cursor_ + 1) % ids_.size();
        ++scanned;
        if (dropped_.count(id) == 0) out.push_back(id);
    }
    return out;
}

void AuxRotation::recordResult(std::uint8_t id, bool ok) {
    if (dropped_.count(id)) return;
    if (ok) {
        fails_[id] = 0;
    } else if (++fails_[id] >= maxFailures_) {
        dropped_.insert(id);
    }
}

std::vector<std::uint8_t> AuxRotation::active() const {
    std::vector<std::uint8_t> out;
    for (std::uint8_t id : ids_) {
        if (dropped_.count(id) == 0) out.push_back(id);
    }
    return out;
}

}  // namespace wp::servo
