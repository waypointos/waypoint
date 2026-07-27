#pragma once

#include <string>

#include "nats/Connection.hpp"

namespace wp::telemetry {

class DrivePublisher {
public:
    DrivePublisher(wp::nats::Connection* nc, const std::string& roverID)
        : nc_(nc), subject_("waypoint." + roverID + ".telemetry.drive") {}

    void publish(double vx_mps, double yawRate_radps);

private:
    wp::nats::Connection* nc_;
    std::string subject_;
};

}  // namespace wp::telemetry
