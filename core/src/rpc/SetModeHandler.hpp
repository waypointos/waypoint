#pragma once

#include <cstdint>
#include <string>

#include "mode/ModeMachine.hpp"
#include "nats/Connection.hpp"

namespace wp::rpc {

// SetModeHandler subscribes to waypoint.<roverID>.rpc.set_mode. The request
// payload is a serialized waypoint::v1::ModeEvent (we reuse the existing
// proto type as Phase 4 introduces no new protocol messages — only the
// `to` field is read). On valid transition the handler replies with a
// ModeEvent reflecting the current mode.
class SetModeHandler {
public:
    SetModeHandler(wp::nats::Connection* nc, wp::mode::ModeMachine* mode,
                   const std::string& roverID);

private:
    wp::nats::Connection* nc_;
    wp::mode::ModeMachine* mode_;
};

}  // namespace wp::rpc
