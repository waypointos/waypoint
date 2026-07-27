#include "servo/MockUart.hpp"

#include <algorithm>
#include <chrono>
#include <thread>

namespace wp::servo {

ssize_t MockUart::write(const std::uint8_t* data, std::size_t n) {
    std::vector<std::uint8_t> req(data, data + n);
    auto resp = respond_(req);
    for (auto b : resp) rx_.push_back(b);
    return static_cast<ssize_t>(n);
}

ssize_t MockUart::read(std::uint8_t* buf, std::size_t n, int timeoutMs) {
    auto deadline = std::chrono::steady_clock::now() + std::chrono::milliseconds(timeoutMs);
    while (rx_.empty()) {
        if (std::chrono::steady_clock::now() >= deadline) return 0;
        std::this_thread::sleep_for(std::chrono::milliseconds(1));
    }
    std::size_t k = std::min(n, rx_.size());
    for (std::size_t i = 0; i < k; ++i) {
        buf[i] = rx_.front();
        rx_.pop_front();
    }
    return static_cast<ssize_t>(k);
}

}  // namespace wp::servo
