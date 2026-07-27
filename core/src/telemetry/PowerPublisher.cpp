#include "telemetry/PowerPublisher.hpp"

#include "messages/power.pb.h"
#include "util/Stamp.hpp"

namespace wp::telemetry {

void PowerPublisher::publish(double busVoltage_v, double currentDraw_a) {
    waypoint::v1::PowerTelemetry msg;
    msg.set_bus_voltage_v(busVoltage_v);
    msg.set_current_draw_a(currentDraw_a);
    wp::util::fillStamp(msg.mutable_stamp());
    *msg.mutable_t() = msg.stamp().t();
    std::string out;
    msg.SerializeToString(&out);
    nc_->publish(subject_, out.data(), out.size());
}

}  // namespace wp::telemetry
