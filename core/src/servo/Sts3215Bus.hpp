#pragma once

#include <cstdint>
#include <memory>
#include <mutex>
#include <optional>
#include <utility>
#include <vector>

#include "servo/Sts3215Frame.hpp"
#include "servo/Uart.hpp"

namespace wp::servo {

struct ServoState {
    std::uint16_t positionRaw = 0;  // 0..4095 (12-bit)
    std::int16_t  speedRaw    = 0;
    std::int16_t  loadRaw     = 0;
    std::uint8_t  voltageDeci = 0;  // ×0.1 V
    std::uint8_t  temperatureC = 0;
    std::uint16_t currentRaw   = 0;
};

class Sts3215Bus {
public:
    explicit Sts3215Bus(std::unique_ptr<Uart> uart) : uart_(std::move(uart)) {}

    int open(const std::string& path, int baud = 1000000) { return uart_->open(path, baud); }
    void close() { uart_->close(); }

    int ping(std::uint8_t id, int timeoutMs = 50);

    // Returns nullopt on timeout / checksum failure / no response.
    std::optional<ServoState> readState(std::uint8_t id);

    int setGoalSpeed(std::uint8_t id, std::int16_t signedSpeed);
    int setTorqueEnable(std::uint8_t id, bool on);

    std::optional<std::uint8_t> readOperatingMode(std::uint8_t id);
    int setOperatingMode(std::uint8_t id, std::uint8_t mode);
    int setEEPROMLock(std::uint8_t id, bool locked);

    int setGoalPosition(std::uint8_t id, std::uint16_t raw);
    int setTorqueLimit(std::uint8_t id, std::uint16_t raw);
    int setAngleLimits(std::uint8_t id, std::uint16_t minRaw, std::uint16_t maxRaw);

    // One-time control-loop tuning. EEPROM-backed fields (returnDelay, P/D/I)
    // share a single unlock/relock cycle; SRAM fields (acceleration,
    // maxAcceleration) need no lock. Only fields that are set are written.
    struct Tuning {
        std::optional<std::uint8_t>  pCoef;
        std::optional<std::uint8_t>  iCoef;
        std::optional<std::uint8_t>  dCoef;
        std::optional<std::uint8_t>  acceleration;
        std::optional<std::uint8_t>  returnDelay;
        std::optional<std::uint16_t> maxAcceleration;
    };
    int setTuning(std::uint8_t id, const Tuning& t);

    int syncWriteGoalPositions(const std::vector<std::pair<std::uint8_t, std::uint16_t>>& goals);

    // Safe-goal latch: in position mode, write GOAL_POSITION = PRESENT_POSITION
    // before enabling torque so the servo cannot snap to a stale goal. In wheel
    // mode (no goal-position behaviour) it just enables torque.
    int enableTorqueHoldingPosition(std::uint8_t id);

private:
    std::optional<std::vector<std::uint8_t>> exchange(const std::vector<std::uint8_t>& req,
                                                      int timeoutMs = 50);
    std::optional<std::vector<std::uint8_t>> exchangeLocked(const std::vector<std::uint8_t>& req,
                                                            int timeoutMs = 50);
    bool sendNoReplyLocked(const std::vector<std::uint8_t>& req);
    std::mutex mu_;
    std::unique_ptr<Uart> uart_;
};

}  // namespace wp::servo
