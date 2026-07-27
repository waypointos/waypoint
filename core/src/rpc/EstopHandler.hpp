#pragma once

#include <string>

#include "mode/ModeMachine.hpp"
#include "nats/Connection.hpp"

namespace wp::rpc {

// EstopHandler subscribes to waypoint.<roverID>.rpc.estop. Any incoming
// message — payload is ignored — forces the mode machine to Estop. Replies
// with a serialized ModeEvent if a reply subject is set.
class EstopHandler {
public:
    EstopHandler(wp::nats::Connection* nc, wp::mode::ModeMachine* mode,
                 const std::string& roverID);

private:
    wp::nats::Connection* nc_;
    wp::mode::ModeMachine* mode_;
};

}  // namespace wp::rpc
