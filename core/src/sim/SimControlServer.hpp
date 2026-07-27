#pragma once

#include <functional>
#include <memory>
#include <string>

#include "messages/sim.pb.h"
#include "nats/Connection.hpp"
#include "sim/ServoModel.hpp"
#include "util/Clock.hpp"

namespace wp::sim {

// NATS surface for the sim: cmd.sim_scenario, rpc.sim_advance, rpc.sim_info.
// Constructed only when the sim driver is active; on hardware the subjects
// have no responder. nc may be null in unit tests (handlers still work).
class SimControlServer {
public:
    SimControlServer(wp::nats::Connection* nc, const std::string& roverID,
                     std::shared_ptr<ServoModel> model, wp::util::SimClock* clock);

    void handleScenario(const waypoint::v1::SimScenario& s);
    waypoint::v1::SimAdvanceResponse handleAdvance(const waypoint::v1::SimAdvanceRequest& r);
    waypoint::v1::SimInfo buildInfo() const;

    // Wired by main when SimUart exists (malformed-frame counter pass-through).
    void setMalformedSource(std::function<std::uint32_t()> src) { malformed_ = std::move(src); }

private:
    wp::nats::Connection* nc_;
    std::shared_ptr<ServoModel> model_;
    wp::util::SimClock* clock_;
    std::function<std::uint32_t()> malformed_;
};

}  // namespace wp::sim
