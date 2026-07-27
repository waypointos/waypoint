#include "servo/Sts3215Bus.hpp"

#include <chrono>
#include <thread>

namespace wp::servo {

int Sts3215Bus::ping(std::uint8_t id, int timeoutMs) {
    auto req = buildPing(id);
    auto resp = exchange(req, timeoutMs);
    return resp ? 0 : -1;
}

std::optional<ServoState> Sts3215Bus::readState(std::uint8_t id) {
    auto req = buildReadStateBlock(id);
    auto resp = exchange(req);
    if (!resp || resp->size() < 1 + 15) return std::nullopt;

    // resp = [err][15 bytes starting at PRESENT_POSITION (0x38..0x46)].
    // PRESENT_CURRENT spans 0x45 (lo) and 0x46 (hi) -> d[13], d[14].
    const std::uint8_t* d = resp->data() + 1;
    ServoState s;
    s.positionRaw   = static_cast<std::uint16_t>(d[0] | (d[1] << 8));
    s.speedRaw      = decodeSignMagnitude(static_cast<std::uint16_t>(d[2] | (d[3] << 8)), 15);
    s.loadRaw       = decodeSignMagnitude(static_cast<std::uint16_t>(d[4] | (d[5] << 8)), 10);
    s.voltageDeci   = d[6];
    s.temperatureC  = d[7];
    s.currentRaw    = static_cast<std::uint16_t>(d[13] | (d[14] << 8));
    return s;
}

int Sts3215Bus::setGoalSpeed(std::uint8_t id, std::int16_t signedSpeed) {
    auto req = buildSetGoalSpeed(id, signedSpeed);
    return exchange(req) ? 0 : -1;
}

int Sts3215Bus::setTorqueEnable(std::uint8_t id, bool on) {
    auto req = buildSetTorqueEnable(id, on);
    return exchange(req) ? 0 : -1;
}

std::optional<std::uint8_t> Sts3215Bus::readOperatingMode(std::uint8_t id) {
    auto req = buildReadMode(id);
    auto resp = exchange(req);
    if (!resp || resp->size() < 2) return std::nullopt;
    return (*resp)[1];
}

int Sts3215Bus::setOperatingMode(std::uint8_t id, std::uint8_t mode) {
    auto req = buildSetMode(id, mode);
    return exchange(req) ? 0 : -1;
}

int Sts3215Bus::setEEPROMLock(std::uint8_t id, bool locked) {
    auto req = buildSetEEPROMLock(id, locked);
    return exchange(req) ? 0 : -1;
}

int Sts3215Bus::setGoalPosition(std::uint8_t id, std::uint16_t raw) {
    return exchange(buildSetGoalPosition(id, raw)) ? 0 : -1;
}

int Sts3215Bus::setTorqueLimit(std::uint8_t id, std::uint16_t raw) {
    return exchange(buildSetTorqueLimit(id, raw)) ? 0 : -1;
}

int Sts3215Bus::setAngleLimits(std::uint8_t id, std::uint16_t minRaw, std::uint16_t maxRaw) {
    std::lock_guard<std::mutex> lk(mu_);
    exchangeLocked(buildSetEEPROMLock(id, false));   // angle limits are EEPROM
    auto ok = exchangeLocked(buildSetAngleLimits(id, minRaw, maxRaw));
    exchangeLocked(buildSetEEPROMLock(id, true));
    return ok ? 0 : -1;
}

int Sts3215Bus::setTuning(std::uint8_t id, const Tuning& t) {
    std::lock_guard<std::mutex> lk(mu_);
    bool ok = true;

    // EEPROM fields (addr < 0x28): write them inside a single unlock/relock
    // cycle, matching setAngleLimits. Skip the cycle entirely if none are set.
    const bool anyEeprom = t.returnDelay || t.pCoef || t.dCoef || t.iCoef;
    if (anyEeprom) {
        exchangeLocked(buildSetEEPROMLock(id, false));
        if (t.returnDelay) ok &= exchangeLocked(buildWriteByte(id, reg::RETURN_DELAY, *t.returnDelay)).has_value();
        if (t.pCoef)       ok &= exchangeLocked(buildWriteByte(id, reg::COMP_P, *t.pCoef)).has_value();
        if (t.dCoef)       ok &= exchangeLocked(buildWriteByte(id, reg::COMP_D, *t.dCoef)).has_value();
        if (t.iCoef)       ok &= exchangeLocked(buildWriteByte(id, reg::COMP_I, *t.iCoef)).has_value();
        exchangeLocked(buildSetEEPROMLock(id, true));
    }

    // SRAM fields: no lock needed.
    if (t.acceleration)    ok &= exchangeLocked(buildWriteByte(id, reg::ACCELERATION, *t.acceleration)).has_value();
    if (t.maxAcceleration) ok &= exchangeLocked(buildWriteWord(id, reg::MAX_ACCELERATION, *t.maxAcceleration)).has_value();

    return ok ? 0 : -1;
}

int Sts3215Bus::enableTorqueHoldingPosition(std::uint8_t id) {
    std::lock_guard<std::mutex> lk(mu_);
    auto modeResp = exchangeLocked(buildReadMode(id));
    bool position = modeResp && modeResp->size() >= 2 && (*modeResp)[1] == 0;  // 0 = position
    if (position) {
        auto st = exchangeLocked(buildReadStateBlock(id));
        if (st && st->size() >= 1 + 15) {
            const std::uint8_t* d = st->data() + 1;
            std::uint16_t pos = static_cast<std::uint16_t>(d[0] | (d[1] << 8));
            exchangeLocked(buildSetGoalPosition(id, pos));
        }
    }
    return exchangeLocked(buildSetTorqueEnable(id, true)) ? 0 : -1;
}

int Sts3215Bus::syncWriteGoalPositions(
    const std::vector<std::pair<std::uint8_t, std::uint16_t>>& goals) {
    if (goals.empty()) return 0;
    std::lock_guard<std::mutex> lk(mu_);
    return sendNoReplyLocked(buildSyncWriteGoalPositions(goals)) ? 0 : -1;
}

bool Sts3215Bus::sendNoReplyLocked(const std::vector<std::uint8_t>& req) {
    uart_->setHalfDuplexDirection(true);   // TX
    bool ok = uart_->write(req.data(), req.size()) >= static_cast<ssize_t>(req.size());
    uart_->setHalfDuplexDirection(false);  // back to RX; broadcast has no reply to read
    return ok;
}

std::optional<std::vector<std::uint8_t>> Sts3215Bus::exchange(const std::vector<std::uint8_t>& req,
                                                              int timeoutMs) {
    std::lock_guard<std::mutex> lk(mu_);
    return exchangeLocked(req, timeoutMs);
}

std::optional<std::vector<std::uint8_t>> Sts3215Bus::exchangeLocked(const std::vector<std::uint8_t>& req,
                                                                    int timeoutMs) {
    uart_->setHalfDuplexDirection(true);  // TX
    if (uart_->write(req.data(), req.size()) < static_cast<ssize_t>(req.size())) return std::nullopt;
    uart_->setHalfDuplexDirection(false);  // RX

    // STS3215 echoes the sent bytes back on the half-duplex line; discard them
    // (the mock uart in tests doesn't echo, so we tolerate both cases).
    std::uint8_t scratch[64];
    auto t0 = std::chrono::steady_clock::now();

    std::vector<std::uint8_t> resp;
    while (true) {
        ssize_t n = uart_->read(scratch, sizeof(scratch), timeoutMs);
        if (n <= 0) return std::nullopt;
        resp.insert(resp.end(), scratch, scratch + n);

        auto dr = decode(resp.data(), resp.size());
        if (dr) {
            // Strip header so callers get [err, params...]
            return std::vector<std::uint8_t>(resp.begin() + 4, resp.begin() + dr->consumed - 1);
        }
        if (std::chrono::steady_clock::now() - t0 > std::chrono::milliseconds(timeoutMs)) {
            return std::nullopt;
        }
    }
}

}  // namespace wp::servo
