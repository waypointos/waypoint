#include "servo/Discovery.hpp"

namespace wp::servo {

std::vector<std::uint8_t> discoverServos(Sts3215Bus& bus, std::uint8_t lo, std::uint8_t hi,
                                         int pingTimeoutMs) {
    std::vector<std::uint8_t> found;
    for (int id = lo; id <= static_cast<int>(hi); ++id) {
        if (bus.ping(static_cast<std::uint8_t>(id), pingTimeoutMs) == 0)
            found.push_back(static_cast<std::uint8_t>(id));
    }
    return found;
}

std::vector<std::uint8_t> discoverServos(Sts3215Bus& bus,
                                         const std::vector<std::uint8_t>& ids,
                                         int pingTimeoutMs) {
    std::vector<std::uint8_t> found;
    for (std::uint8_t id : ids) {
        if (bus.ping(id, pingTimeoutMs) == 0) found.push_back(id);
    }
    return found;
}

}  // namespace wp::servo
