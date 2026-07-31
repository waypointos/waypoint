#include "rpc/EstopHandler.hpp"

#include "messages/common.pb.h"
#include "messages/events.pb.h"

namespace wp::rpc {

namespace {
waypoint::v1::Mode toProto(wp::mode::Mode m) {
    switch (m) {
    case wp::mode::Mode::Manual:     return waypoint::v1::MODE_MANUAL;
    case wp::mode::Mode::Safe:       return waypoint::v1::MODE_SAFE;
    case wp::mode::Mode::Estop:      return waypoint::v1::MODE_ESTOP;
    case wp::mode::Mode::Autonomous: return waypoint::v1::MODE_AUTONOMOUS;
    }
    return waypoint::v1::MODE_UNSPECIFIED;
}
}  // namespace

EstopHandler::EstopHandler(wp::nats::Connection* nc, wp::mode::ModeMachine* mode,
                           const std::string& roverID)
    : nc_(nc), mode_(mode) {
    const std::string subject = "waypoint." + roverID + ".rpc.estop";
    nc_->subscribe(subject, [this](const wp::nats::Message& m) {
        auto before = mode_->current();
        mode_->requestEstop();

        if (!m.reply.empty()) {
            waypoint::v1::ModeEvent resp;
            resp.set_from(toProto(before));
            resp.set_to(toProto(mode_->current()));
            std::string out;
            resp.SerializeToString(&out);
            nc_->publish(m.reply, out.data(), out.size());
        }
    });
}

}  // namespace wp::rpc
