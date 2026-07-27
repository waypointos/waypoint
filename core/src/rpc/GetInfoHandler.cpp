#include "rpc/GetInfoHandler.hpp"

#include "messages/system.pb.h"

namespace wp::rpc {

GetInfoHandler::GetInfoHandler(wp::nats::Connection* nc, const std::string& roverID,
                               std::string imageVersion)
    : nc_(nc), imageVersion_(std::move(imageVersion)) {
    const std::string subject = "waypoint." + roverID + ".rpc.get_info";
    nc_->subscribe(subject, [this](const wp::nats::Message& m) {
        if (m.reply.empty()) return;
        waypoint::v1::SystemTelemetry resp;
        resp.set_image_version(imageVersion_);
        std::string out;
        resp.SerializeToString(&out);
        nc_->publish(m.reply, out.data(), out.size());
    });
}

}  // namespace wp::rpc
