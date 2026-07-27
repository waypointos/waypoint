#pragma once

#include <string>

#include "mode/ModeMachine.hpp"
#include "nats/Connection.hpp"

namespace wp::rpc {

// RecoverHandler subscribes to waypoint.<roverID>.rpc.recover. Any incoming
// message — payload is ignored — asks the mode machine to clear an engaged
// E-stop (Estop -> Manual if heartbeat is live, else Safe). Replies with a
// serialized ModeEvent if a reply subject is set.
class RecoverHandler {
public:
    RecoverHandler(wp::nats::Connection* nc, wp::mode::ModeMachine* mode,
                   const std::string& roverID);

private:
    wp::nats::Connection* nc_;
    wp::mode::ModeMachine* mode_;
};

}  // namespace wp::rpc
