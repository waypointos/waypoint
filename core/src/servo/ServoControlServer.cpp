#include "servo/ServoControlServer.hpp"

#include "util/Stamp.hpp"

namespace wp::servo {

ServoControlServer::ServoControlServer(wp::nats::Connection* nc, Sts3215Bus* bus,
                                       std::function<bool()> isEstopped,
                                       const std::string& roverID,
                                       std::vector<std::uint8_t> denyIds)
    : nc_(nc), bus_(bus), isEstopped_(std::move(isEstopped)), deny_(std::move(denyIds)) {
    if (nc_) {
        nc_->subscribe("waypoint." + roverID + ".cmd.servo", [this](const wp::nats::Message& m) {
            waypoint::v1::ServoControl c;
            if (!c.ParseFromString(m.payload)) return;
            handleControl(c);
        });
        nc_->subscribe("waypoint." + roverID + ".rpc.servo_read", [this](const wp::nats::Message& m) {
            if (m.reply.empty()) return;
            waypoint::v1::ServoReadRequest req;
            if (!req.ParseFromString(m.payload)) return;
            waypoint::v1::ServoState resp = handleRead(req.servo_id());
            std::string out;
            resp.SerializeToString(&out);
            nc_->publish(m.reply, out.data(), out.size());
        });
        nc_->subscribe("waypoint." + roverID + ".cmd.servo_sync", [this](const wp::nats::Message& m) {
            waypoint::v1::ServoSyncWrite s;
            if (!s.ParseFromString(m.payload)) return;
            handleSync(s);
        });
    }
}

bool ServoControlServer::isDenied(std::uint32_t id) const {
    return std::find(deny_.begin(), deny_.end(), static_cast<std::uint8_t>(id)) != deny_.end();
}

void ServoControlServer::handleControl(const waypoint::v1::ServoControl& c) {
    const std::uint32_t id = c.servo_id();
    if (id < 1 || id > 12) return;
    if (isDenied(id)) return;                     // platform-owned servo guard
    if (isEstopped_ && isEstopped_()) return;     // safety gate
    const auto id8 = static_cast<std::uint8_t>(id);

    switch (c.op_case()) {
    case waypoint::v1::ServoControl::kSetMode:
        bus_->setEEPROMLock(id8, false);
        bus_->setOperatingMode(id8, static_cast<std::uint8_t>(c.set_mode()));
        bus_->setEEPROMLock(id8, true);
        break;
    case waypoint::v1::ServoControl::kSetTorqueEnable:
        if (c.set_torque_enable()) bus_->enableTorqueHoldingPosition(id8);
        else bus_->setTorqueEnable(id8, false);
        break;
    case waypoint::v1::ServoControl::kSetGoalPosition:
        bus_->setGoalPosition(id8, static_cast<std::uint16_t>(c.set_goal_position() & 0x0FFF));
        break;
    case waypoint::v1::ServoControl::kSetTorqueLimit:
        bus_->setTorqueLimit(id8, static_cast<std::uint16_t>(c.set_torque_limit()));
        break;
    case waypoint::v1::ServoControl::kSetAngleLimits:
        bus_->setAngleLimits(id8, static_cast<std::uint16_t>(c.set_angle_limits().min_raw()),
                             static_cast<std::uint16_t>(c.set_angle_limits().max_raw()));
        break;
    case waypoint::v1::ServoControl::kSetOvercurrentLimit: {
        std::lock_guard<std::mutex> lk(ceilMu_);
        overcurrentCeil_[id] = static_cast<std::uint16_t>(c.set_overcurrent_limit());
        break;
    }
    case waypoint::v1::ServoControl::kSetGoalSpeed:
        // Position-mode moving-speed cap. The register is sign-magnitude; in
        // position mode the servo uses the magnitude as its max speed to goal.
        bus_->setGoalSpeed(id8, static_cast<std::int16_t>(c.set_goal_speed() & 0x7FFF));
        break;
    case waypoint::v1::ServoControl::kSetGoalVelocity: {
        // Wheel-mode signed velocity. The bus encodes sign-magnitude, so the
        // value is clamped rather than masked to keep the direction bit.
        std::int32_t v = c.set_goal_velocity();
        if (v > 32767) v = 32767;
        if (v < -32767) v = -32767;
        bus_->setGoalSpeed(id8, static_cast<std::int16_t>(v));
        break;
    }
    case waypoint::v1::ServoControl::kSetTuning: {
        // One-time control-loop tuning. Map only the present proto optionals so
        // unset fields are left untouched on the servo.
        const auto& t = c.set_tuning();
        Sts3215Bus::Tuning tn;
        if (t.has_p_coefficient())    tn.pCoef = static_cast<std::uint8_t>(t.p_coefficient());
        if (t.has_i_coefficient())    tn.iCoef = static_cast<std::uint8_t>(t.i_coefficient());
        if (t.has_d_coefficient())    tn.dCoef = static_cast<std::uint8_t>(t.d_coefficient());
        if (t.has_acceleration())     tn.acceleration = static_cast<std::uint8_t>(t.acceleration());
        if (t.has_return_delay())     tn.returnDelay = static_cast<std::uint8_t>(t.return_delay());
        if (t.has_max_acceleration()) tn.maxAcceleration = static_cast<std::uint16_t>(t.max_acceleration());
        bus_->setTuning(id8, tn);
        break;
    }
    case waypoint::v1::ServoControl::OP_NOT_SET:
    default:
        break;
    }
}

void ServoControlServer::handleSync(const waypoint::v1::ServoSyncWrite& s) {
    if (isEstopped_ && isEstopped_()) return;             // ESTOP GATE
    std::vector<std::pair<std::uint8_t, std::uint16_t>> goals;
    for (const auto& g : s.goals()) {
        const std::uint32_t id = g.servo_id();
        if (id < 1 || id > 12) continue;
        if (isDenied(id)) continue;                       // PLATFORM-OWNED SERVO GUARD
        goals.emplace_back(static_cast<std::uint8_t>(id),
                           static_cast<std::uint16_t>(g.goal_position() & 0x0FFF));
    }
    if (!goals.empty()) bus_->syncWriteGoalPositions(goals);
}

waypoint::v1::ServoState ServoControlServer::handleRead(std::uint32_t servoID) {
    waypoint::v1::ServoState out;
    out.set_servo_id(servoID);
    if (servoID < 1 || servoID > 12) { out.set_ok(false); return out; }

    auto st = bus_->readState(static_cast<std::uint8_t>(servoID));
    if (!st) { out.set_ok(false); return out; }

    out.set_ok(true);
    out.set_position_raw(st->positionRaw);
    out.set_speed_raw(st->speedRaw);
    out.set_load_raw(st->loadRaw);
    out.set_current_raw(st->currentRaw);
    out.set_voltage_deci(st->voltageDeci);
    out.set_temperature_c(st->temperatureC);
    out.set_moving(st->speedRaw != 0);

    std::uint16_t ceil;
    { std::lock_guard<std::mutex> lk(ceilMu_); ceil = overcurrentCeil_[servoID]; }
    if (ceil > 0 && st->currentRaw > ceil) {
        bus_->setTorqueEnable(static_cast<std::uint8_t>(servoID), false);
        out.set_overcurrent_tripped(true);
    }
    wp::util::fillStamp(out.mutable_stamp());
    return out;
}

}  // namespace wp::servo
