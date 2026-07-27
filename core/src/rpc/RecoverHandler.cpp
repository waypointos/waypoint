#include "rpc/RecoverHandler.hpp"

#include "messages/common.pb.h"
#include "messages/events.pb.h"

namespace wp::rpc {

namespace {
waypoint::v1::Mode toProto(wp::mode::Mode m) {
    switch (m) {
    case wp::mode::Mode::Manual: return waypoint::v1::MODE_MANUAL;
    case wp::mode::Mode::Safe:   return waypoint::v1::MODE_SAFE;
    case wp::mode::Mode::Estop:  return waypoint::v1::MODE_ESTOP;
    }
    return waypoint::v1::MODE_UNSPECIFIED;
}
}  // namespace

RecoverHandler::RecoverHandler(wp::nats::Connection* nc, wp::mode::ModeMachine* mode,
                               const std::string& roverID)
    : nc_(nc), mode_(mode) {
    const std::string subject = "waypoint." + roverID + ".rpc.recover";
    nc_->subscribe(subject, [this](const wp::nats::Message& m) {
        auto before = mode_->current();
        mode_->requestRecover();

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
