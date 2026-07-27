#pragma once

#include <string>

#include "messages/platform.pb.h"
#include "nats/Connection.hpp"
#include "platform/Descriptor.hpp"

namespace wp::telemetry {

// Projects the loaded descriptor onto waypoint.v1.PlatformInfo. Free function
// so the projection is unit-testable without a NATS connection.
waypoint::v1::PlatformInfo buildPlatformInfo(const wp::platform::Descriptor& d);

// Publishes the projection on infra.platform at boot and on a 5 s
// re-announce cadence (content is static per boot; only the stamp moves).
class PlatformPublisher {
public:
    PlatformPublisher(wp::nats::Connection* nc, const std::string& roverID,
                      const wp::platform::Descriptor& desc)
        : nc_(nc),
          subject_("waypoint." + roverID + ".infra.platform"),
          msg_(buildPlatformInfo(desc)) {}

    void publish();

private:
    wp::nats::Connection* nc_;
    std::string subject_;
    waypoint::v1::PlatformInfo msg_;
};

}  // namespace wp::telemetry
