#pragma once

#include <atomic>
#include <cstdint>
#include <deque>
#include <memory>
#include <vector>

#include "servo/Sts3215Frame.hpp"
#include "servo/Uart.hpp"
#include "sim/ServoModel.hpp"

namespace wp::sim {

// Behavioral sim backend under the byte seam: parses STS3215 frames, applies
// them to the ServoModel, queues half-duplex echo plus synthesized responses.
// Selected when the descriptor driver kind is "sim" (or --servo-mock).
class SimUart : public servo::Uart {
public:
    explicit SimUart(std::shared_ptr<ServoModel> model);

    int open(const std::string&, int) override { return 0; }
    void close() override {}
    ssize_t write(const std::uint8_t* data, std::size_t n) override;
    ssize_t read(std::uint8_t* buf, std::size_t n, int timeoutMs) override;
    // Half-duplex direction gates the echo: bytes written while the line is in
    // TX never reach RX (the transceiver is driving), matching the real wiring.
    void setHalfDuplexDirection(bool tx) override { txMode_ = tx; }

    // Read from the NATS thread via rpc.sim_info while the loop thread writes.
    std::uint32_t malformedFrames() const { return malformed_.load(); }

private:
    void dispatch(const servo::Frame& f);
    void respond(std::uint8_t id, const std::vector<std::uint8_t>& params);

    std::shared_ptr<ServoModel> model_;
    std::vector<std::uint8_t> txAccum_;
    std::deque<std::uint8_t> rx_;
    std::atomic<std::uint32_t> malformed_{0};
    bool txMode_ = false;  // false = RX (default), set true by the bus during write
};

}  // namespace wp::sim
