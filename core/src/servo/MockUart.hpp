#pragma once

#include <deque>
#include <functional>
#include <vector>

#include "servo/Uart.hpp"

namespace wp::servo {

// MockUart loops back: when the caller writes a request frame, MockUart
// optionally synthesizes a reply (via the callback) and queues it to be
// read back. Used for unit-testing the servo bus driver and integration-
// testing the closed-loop controller against a synthetic plant.
class MockUart : public Uart {
public:
    using ResponseFn = std::function<std::vector<std::uint8_t>(const std::vector<std::uint8_t>&)>;

    explicit MockUart(ResponseFn fn) : respond_(std::move(fn)) {}

    int open(const std::string&, int) override { return 0; }
    void close() override {}
    ssize_t write(const std::uint8_t* data, std::size_t n) override;
    ssize_t read(std::uint8_t* buf, std::size_t n, int timeoutMs) override;

private:
    ResponseFn respond_;
    std::deque<std::uint8_t> rx_;
};

}  // namespace wp::servo
