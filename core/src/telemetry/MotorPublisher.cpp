#include "telemetry/MotorPublisher.hpp"

#include <cmath>

#include "messages/motors.pb.h"
#include "util/Stamp.hpp"

namespace wp::telemetry {

namespace {
constexpr double kStepsPerRev = 4096.0;
constexpr double kTwoPi = 2.0 * 3.14159265358979;
}  // namespace

void MotorPublisher::publish(std::uint8_t id, const servo::ServoState& s) {
    waypoint::v1::MotorTelemetry msg;
    msg.set_id(id);
    wp::util::fillStamp(msg.mutable_stamp());
    *msg.mutable_t() = msg.stamp().t();
    msg.set_position_rad(static_cast<double>(s.positionRaw) * (kTwoPi / kStepsPerRev));
    msg.set_velocity_radps(static_cast<double>(s.speedRaw) * (kTwoPi / kStepsPerRev));
    msg.set_current_a(static_cast<double>(s.currentRaw) * 0.0065);  // datasheet: 6.5 mA / unit
    msg.set_temperature_c(static_cast<double>(s.temperatureC));
    msg.set_bus_voltage_v(static_cast<double>(s.voltageDeci) * 0.1);
    std::string out;
    msg.SerializeToString(&out);
    nc_->publish(subject_, out.data(), out.size());
}

}  // namespace wp::telemetry
