#include "telemetry/DrivePublisher.hpp"

#include "messages/drive.pb.h"
#include "util/Stamp.hpp"

namespace wp::telemetry {

void DrivePublisher::publish(double vx_mps, double yawRate_radps) {
    waypoint::v1::DriveTelemetry msg;
    msg.set_body_vx_mps(vx_mps);
    msg.set_yaw_rate_radps(yawRate_radps);
    wp::util::fillStamp(msg.mutable_stamp());
    *msg.mutable_t() = msg.stamp().t();
    std::string out;
    msg.SerializeToString(&out);
    nc_->publish(subject_, out.data(), out.size());
}

}  // namespace wp::telemetry
