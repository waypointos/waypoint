#pragma once

#include <string>

#include "nats/Connection.hpp"

namespace wp::rpc {

// GetInfoHandler subscribes to waypoint.<roverID>.rpc.get_info and responds
// with a SystemTelemetry message describing the running daemon. Phase 4 has
// no dedicated InfoResponse type, so SystemTelemetry (image_version field)
// stands in until the protocol gains a proper info message.
class GetInfoHandler {
public:
    GetInfoHandler(wp::nats::Connection* nc, const std::string& roverID,
                   std::string imageVersion);

private:
    wp::nats::Connection* nc_;
    std::string imageVersion_;
};

}  // namespace wp::rpc
