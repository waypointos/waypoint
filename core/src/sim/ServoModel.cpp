#include "sim/ServoModel.hpp"

#include <algorithm>
#include <cmath>

#include "servo/Sts3215Frame.hpp"

namespace wp::sim {

namespace {
constexpr double kTwoPi = 2.0 * 3.14159265358979;
constexpr double kStepsPerRev = 4096.0;
constexpr double kTau = 0.05;            // velocity first-order time constant
constexpr double kStallTau = 0.1;        // current ramp at a hard stop
constexpr double kIdleCurrentA = 0.5;
constexpr double kCurrentPerRadps = 0.4;
constexpr double kBusV0 = 12.4;
constexpr double kSagPerAmp = 0.04;
constexpr double kHeatPerAmp2 = 0.02;    // deg C per A^2 per second
constexpr double kCoolPerSec = 0.05;     // fraction toward ambient per second
constexpr double kAmbientC = 25.0;
constexpr double kPositionSpeedRadps = 3.0;  // position-mode travel speed
constexpr double kCurrentToRaw = 1.0 / 0.0065;
constexpr std::size_t kRegCount = 0x50;

double radpsFromRawSpeed(std::int16_t raw) { return raw * (kTwoPi / kStepsPerRev); }
double rawSpeedFromRadps(double radps) { return radps / (kTwoPi / kStepsPerRev); }

// GOAL_SPEED arrives encoded by the real builder (buildSetGoalSpeed):
// sign-magnitude with bit 15 as the direction flag, matching the servo.
std::int16_t decodeGoalSpeed(const std::uint8_t* lo) {
    return servo::decodeSignMagnitude(static_cast<std::uint16_t>(lo[0] | (lo[1] << 8)), 15);
}

std::uint16_t readU16(const std::array<std::uint8_t, kRegCount>& regs, std::uint8_t addr) {
    return static_cast<std::uint16_t>(regs[addr] | (regs[addr + 1] << 8));
}
void writeU16(std::array<std::uint8_t, kRegCount>& regs, std::uint8_t addr, std::uint16_t v) {
    regs[addr] = static_cast<std::uint8_t>(v & 0xFF);
    regs[addr + 1] = static_cast<std::uint8_t>((v >> 8) & 0xFF);
}
}  // namespace

ServoModel::ServoModel(std::vector<ServoConfig> servos, std::uint64_t seed)
    : seed_(seed), rng_(seed) {
    for (const auto& cfg : servos) {
        Servo s;
        s.cfg = cfg;
        s.regs[servo::reg::MODE] = cfg.wheelMode ? 1 : 0;
        mirrorPresentRegs(s);
        servos_[cfg.id] = s;
    }
}

ServoModel::Servo* ServoModel::find(std::uint8_t id) {
    auto it = servos_.find(id);
    return it == servos_.end() ? nullptr : &it->second;
}
const ServoModel::Servo* ServoModel::find(std::uint8_t id) const {
    auto it = servos_.find(id);
    return it == servos_.end() ? nullptr : &it->second;
}

bool ServoModel::declared(std::uint8_t id) const {
    std::lock_guard<std::mutex> lk(mu_);
    return find(id) != nullptr;
}

bool ServoModel::present(std::uint8_t id) const {
    std::lock_guard<std::mutex> lk(mu_);
    const Servo* s = find(id);
    return s && s->cfg.present;
}

bool ServoModel::writeReg(std::uint8_t id, std::uint8_t addr, const std::uint8_t* data,
                          std::size_t n) {
    std::lock_guard<std::mutex> lk(mu_);
    Servo* s = find(id);
    if (!s || !s->cfg.present) return false;
    if (n == 0 || data == nullptr) return false;
    if (static_cast<std::size_t>(addr) + n > kRegCount) return false;
    // EEPROM addresses below TORQUE_ENABLE are frozen while LOCK == 1; LOCK
    // itself stays writable so the lock can be released.
    const bool locked = s->regs[servo::reg::LOCK] == 1;
    if (locked && addr < servo::reg::TORQUE_ENABLE && addr != servo::reg::LOCK) {
        return false;
    }
    for (std::size_t i = 0; i < n; ++i) s->regs[addr + i] = data[i];
    return true;
}

std::optional<std::vector<std::uint8_t>> ServoModel::readRegs(std::uint8_t id, std::uint8_t addr,
                                                              std::uint8_t len) {
    std::lock_guard<std::mutex> lk(mu_);
    Servo* s = find(id);
    if (!s || !s->cfg.present) return std::nullopt;
    if (static_cast<std::size_t>(addr) + len > kRegCount) return std::nullopt;
    return std::vector<std::uint8_t>(s->regs.begin() + addr, s->regs.begin() + addr + len);
}

void ServoModel::step(double dt) {
    std::lock_guard<std::mutex> lk(mu_);
    double summedCurrent = 0.0;
    for (auto& [id, s] : servos_) {
        if (!s.cfg.present) continue;
        const bool wheel = s.regs[servo::reg::MODE] == 1;
        const bool torque = s.regs[servo::reg::TORQUE_ENABLE] == 1;
        bool stalled = false;

        if (wheel) {
            // Wheel mode acts on GOAL_SPEED regardless of torque (the quirk).
            std::int16_t raw = decodeGoalSpeed(&s.regs[servo::reg::GOAL_SPEED]);
            double target = radpsFromRawSpeed(raw);
            double k = std::min(1.0, dt / kTau);
            s.velocityRadps += (target - s.velocityRadps) * k;
            s.positionTicks += rawSpeedFromRadps(s.velocityRadps) * dt;
        } else if (torque) {
            // Position mode: travel toward GOAL_POSITION at a fixed speed.
            double goal = static_cast<double>(readU16(s.regs, servo::reg::GOAL_POSITION));
            double maxStep = rawSpeedFromRadps(kPositionSpeedRadps) * dt;
            double delta = goal - s.positionTicks;
            double moved = std::clamp(delta, -maxStep, maxStep);
            s.positionTicks += moved;
            s.velocityRadps = radpsFromRawSpeed(static_cast<std::int16_t>(moved / dt));
        } else {
            s.velocityRadps = 0.0;
        }

        // Hard stops clamp motion; commanding past a stop pins the position
        // and ramps current toward the stall value while velocity reads 0.
        if (s.hardStops) {
            double lo = s.hardStops->first;
            double hi = s.hardStops->second;
            if (s.positionTicks < lo || s.positionTicks > hi) {
                s.positionTicks = std::clamp(s.positionTicks, lo, hi);
                s.velocityRadps = 0.0;
                stalled = true;
            }
        }

        // Wrap into [0, 4096) only outside a hard-stop window (the encoder seam).
        if (!s.hardStops) {
            s.positionTicks = std::fmod(s.positionTicks, kStepsPerRev);
            if (s.positionTicks < 0) s.positionTicks += kStepsPerRev;
        }

        double targetCurrent =
            stalled ? s.stallCurrentA
                    : kIdleCurrentA + kCurrentPerRadps * std::abs(s.velocityRadps);
        double kc = std::min(1.0, dt / kStallTau);
        s.currentA += (targetCurrent - s.currentA) * kc;
        summedCurrent += s.currentA;

        // Temperature integrates with current^2 and relaxes toward ambient.
        s.temperatureC += kHeatPerAmp2 * s.currentA * s.currentA * dt;
        double ambient = kAmbientC + s.temperatureOffsetC;
        s.temperatureC += (ambient - s.temperatureC) * kCoolPerSec * dt;
    }

    std::uniform_real_distribution<double> noise(-0.02, 0.02);
    busVoltageV_ = kBusV0 - voltageSagFactor_ * kSagPerAmp * summedCurrent + noise(rng_);

    for (auto& [id, s] : servos_) {
        if (s.cfg.present) mirrorPresentRegs(s);
    }
}

void ServoModel::mirrorPresentRegs(Servo& s) {
    writeU16(s.regs, servo::reg::PRESENT_POSITION,
             static_cast<std::uint16_t>(std::lround(s.positionTicks)) & 0x0FFF);
    const long speedRaw = std::clamp(std::lround(rawSpeedFromRadps(s.velocityRadps)),
                                     -32767L, 32767L);
    writeU16(s.regs, servo::reg::PRESENT_SPEED,
             servo::encodeSignMagnitude(static_cast<std::int16_t>(speedRaw), 15));
    writeU16(s.regs, servo::reg::PRESENT_LOAD, 0);
    s.regs[servo::reg::PRESENT_VOLTAGE] =
        static_cast<std::uint8_t>(std::clamp(std::lround(busVoltageV_ * 10.0), 0L, 255L));
    s.regs[servo::reg::PRESENT_TEMPERATURE] =
        static_cast<std::uint8_t>(std::clamp(std::lround(s.temperatureC), 0L, 255L));
    writeU16(s.regs, servo::reg::PRESENT_CURRENT,
             static_cast<std::uint16_t>(std::lround(s.currentA * kCurrentToRaw)));
}

void ServoModel::setAbsent(std::uint8_t id, bool absent) {
    std::lock_guard<std::mutex> lk(mu_);
    if (Servo* s = find(id)) {
        s->cfg.present = !absent;
        s->overridden = true;
    }
}

void ServoModel::setHardStops(std::uint8_t id, std::uint16_t minTicks, std::uint16_t maxTicks) {
    std::lock_guard<std::mutex> lk(mu_);
    if (Servo* s = find(id)) {
        s->hardStops = std::make_pair(minTicks, maxTicks);
        s->overridden = true;
    }
}

void ServoModel::setStallCurrent(std::uint8_t id, double amps) {
    std::lock_guard<std::mutex> lk(mu_);
    if (Servo* s = find(id)) {
        s->stallCurrentA = amps;
        s->overridden = true;
    }
}

void ServoModel::setTemperatureOffset(std::uint8_t id, double c) {
    std::lock_guard<std::mutex> lk(mu_);
    if (Servo* s = find(id)) {
        s->temperatureOffsetC = c;
        s->overridden = true;
    }
}

void ServoModel::setVoltageSagFactor(double f) {
    std::lock_guard<std::mutex> lk(mu_);
    voltageSagFactor_ = f;
}

void ServoModel::resetSeed(std::uint64_t seed) {
    std::lock_guard<std::mutex> lk(mu_);
    seed_ = seed;
    rng_.seed(seed);
}

std::uint64_t ServoModel::seed() const {
    std::lock_guard<std::mutex> lk(mu_);
    return seed_;
}

void ServoModel::clearOverrides() {
    std::lock_guard<std::mutex> lk(mu_);
    for (auto& [id, s] : servos_) {
        s.cfg.present = true;
        s.hardStops.reset();
        s.stallCurrentA = 2.0;
        s.temperatureOffsetC = 0.0;
        s.overridden = false;
    }
    voltageSagFactor_ = 1.0;
}

std::uint32_t ServoModel::overrideCount() const {
    std::lock_guard<std::mutex> lk(mu_);
    std::uint32_t n = 0;
    for (const auto& [id, s] : servos_) {
        if (s.overridden) ++n;
    }
    return n;
}

}  // namespace wp::sim
