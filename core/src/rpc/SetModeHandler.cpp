#include "rpc/SetModeHandler.hpp"

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

wp::mode::Mode fromProto(waypoint::v1::Mode p) {
    switch (p) {
    case waypoint::v1::MODE_MANUAL: return wp::mode::Mode::Manual;
    case waypoint::v1::MODE_SAFE:   return wp::mode::Mode::Safe;
    case waypoint::v1::MODE_ESTOP:  return wp::mode::Mode::Estop;
    default:                        return wp::mode::Mode::Safe;
    }
}
}  // namespace

SetModeHandler::SetModeHandler(wp::nats::Connection* nc, wp::mode::ModeMachine* mode,
                               const std::string& roverID)
    : nc_(nc), mode_(mode) {
    const std::string subject = "waypoint." + roverID + ".rpc.set_mode";
    nc_->subscribe(subject, [this](const wp::nats::Message& m) {
        waypoint::v1::ModeEvent req;
        if (!req.ParseFromString(m.payload)) return;

        auto target = fromProto(req.to());
        auto before = mode_->current();
        bool ok = (target == wp::mode::Mode::Estop)
                      ? mode_->requestEstop()
                      : mode_->requestSetMode(target);
        (void)ok;

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
