#pragma once

#include <string>

#include "nats/Connection.hpp"

namespace wp::telemetry {

class PowerPublisher {
public:
    PowerPublisher(wp::nats::Connection* nc, const std::string& roverID)
        : nc_(nc), subject_("waypoint." + roverID + ".telemetry.power") {}

    void publish(double busVoltage_v, double currentDraw_a);

private:
    wp::nats::Connection* nc_;
    std::string subject_;
};

}  // namespace wp::telemetry
