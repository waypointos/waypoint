#include "joint/Sts3215Driver.hpp"

namespace wp::joint {

namespace {
constexpr double kTwoPi = 2.0 * 3.14159265358979;
constexpr double kStepsPerRev = 4096.0;
constexpr std::uint8_t kModeWheel = 1;
constexpr std::uint8_t kModePosition = 0;

std::int16_t rawFromRadps(double radps) {
    double v = radps / (kTwoPi / kStepsPerRev);
    // Sign-magnitude wire format: magnitude must fit in 15 bits.
    if (v > 32767) v = 32767;
    if (v < -32767) v = -32767;
    return static_cast<std::int16_t>(v);
}
double radpsFromRaw(std::int16_t raw) { return raw * (kTwoPi / kStepsPerRev); }

std::uint16_t ticksFromRad(double rad) {
    double v = rad / (kTwoPi / kStepsPerRev);
    if (v < 0) v = 0;
    if (v > 4095) v = 4095;
    return static_cast<std::uint16_t>(v);
}
}  // namespace

Sts3215Driver::Sts3215Driver(servo::Sts3215Bus* bus, const platform::Descriptor& d,
                             const std::string& driverName)
    : bus_(bus) {
    for (const auto& j : d.joints) {
        if (j.driver != driverName) continue;
        byName_[j.name] = j.busId;
        info_[j.busId] = Info{.invert = j.invert, .isWheel = j.type == "wheel"};
    }
}

std::optional<JointId> Sts3215Driver::idForName(const std::string& name) const {
    auto it = byName_.find(name);
    if (it == byName_.end()) return std::nullopt;
    return it->second;
}

int Sts3215Driver::ensureModes(const std::vector<JointId>& presentIds) {
    int ok = 0;
    for (JointId id : presentIds) {
        auto it = info_.find(id);
        if (it == info_.end()) continue;
        std::uint8_t desired = it->second.isWheel ? kModeWheel : kModePosition;
        auto cur = bus_->readOperatingMode(id);
        if (cur && *cur == desired) { ++ok; continue; }
        bus_->setTorqueEnable(id, false);
        bus_->setEEPROMLock(id, false);
        int e = bus_->setOperatingMode(id, desired);
        bus_->setEEPROMLock(id, true);
        if (e == 0) ++ok;
    }
    return ok;
}

int Sts3215Driver::setTorqueEnable(JointId id, bool on) {
    return on ? bus_->enableTorqueHoldingPosition(id) : bus_->setTorqueEnable(id, false);
}

void Sts3215Driver::writeCommands(const std::vector<std::pair<JointId, JointCommand>>& cmds) {
    for (const auto& [id, cmd] : cmds) {
        auto it = info_.find(id);
        if (it == info_.end()) continue;
        double sign = it->second.invert ? -1.0 : 1.0;
        if (cmd.velocityRadps) {
            bus_->setGoalSpeed(id, rawFromRadps(sign * *cmd.velocityRadps));
        }
        if (cmd.positionRad) {
            bus_->setGoalPosition(id, ticksFromRad(*cmd.positionRad));
        }
    }
}

std::optional<JointState> Sts3215Driver::readState(JointId id) {
    auto it = info_.find(id);
    if (it == info_.end()) return std::nullopt;
    auto s = bus_->readState(id);
    if (!s) return std::nullopt;
    double sign = it->second.invert ? -1.0 : 1.0;
    JointState out;
    out.positionRad = s->positionRaw * (kTwoPi / kStepsPerRev);
    out.velocityRadps = sign * radpsFromRaw(s->speedRaw);
    out.load = static_cast<double>(s->loadRaw);
    out.currentA = s->currentRaw * 0.0065;  // datasheet: 6.5 mA / unit
    out.voltageV = s->voltageDeci * 0.1;
    out.temperatureC = static_cast<double>(s->temperatureC);
    return out;
}

}  // namespace wp::joint
