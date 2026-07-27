#include "sim/SimControlServer.hpp"

#include <iostream>

namespace wp::sim {

SimControlServer::SimControlServer(wp::nats::Connection* nc, const std::string& roverID,
                                   std::shared_ptr<ServoModel> model, wp::util::SimClock* clock)
    : nc_(nc), model_(std::move(model)), clock_(clock) {
    if (nc_) {
        nc_->subscribe("waypoint." + roverID + ".cmd.sim_scenario",
                       [this](const wp::nats::Message& m) {
                           waypoint::v1::SimScenario s;
                           if (!s.ParseFromString(m.payload)) return;
                           handleScenario(s);
                       });
        nc_->subscribe("waypoint." + roverID + ".rpc.sim_advance",
                       [this](const wp::nats::Message& m) {
                           if (m.reply.empty()) return;
                           waypoint::v1::SimAdvanceRequest req;
                           if (!req.ParseFromString(m.payload)) return;
                           waypoint::v1::SimAdvanceResponse resp = handleAdvance(req);
                           std::string out;
                           resp.SerializeToString(&out);
                           nc_->publish(m.reply, out.data(), out.size());
                       });
        nc_->subscribe("waypoint." + roverID + ".rpc.sim_info",
                       [this](const wp::nats::Message& m) {
                           if (m.reply.empty()) return;
                           waypoint::v1::SimInfo resp = buildInfo();
                           std::string out;
                           resp.SerializeToString(&out);
                           nc_->publish(m.reply, out.data(), out.size());
                       });
    }
}

void SimControlServer::handleScenario(const waypoint::v1::SimScenario& s) {
    if (s.has_clear() && s.clear()) model_->clearOverrides();
    if (s.has_reset_seed()) model_->resetSeed(s.reset_seed());
    if (s.has_voltage_sag_factor()) model_->setVoltageSagFactor(s.voltage_sag_factor());

    if (!s.has_servo_id()) return;
    const auto id = static_cast<std::uint8_t>(s.servo_id());
    if (!model_->declared(id)) {
        std::cerr << "waypoint-core: sim_scenario for undeclared servo "
                  << static_cast<unsigned>(id) << " ignored\n";
        return;
    }
    if (s.has_absent()) model_->setAbsent(id, s.absent());
    if (s.has_hard_stop_min_ticks() || s.has_hard_stop_max_ticks()) {
        model_->setHardStops(id, static_cast<std::uint16_t>(s.hard_stop_min_ticks()),
                             static_cast<std::uint16_t>(s.hard_stop_max_ticks()));
    }
    if (s.has_stall_current_a()) model_->setStallCurrent(id, s.stall_current_a());
    if (s.has_temperature_offset_c()) model_->setTemperatureOffset(id, s.temperature_offset_c());
}

waypoint::v1::SimAdvanceResponse SimControlServer::handleAdvance(
    const waypoint::v1::SimAdvanceRequest& r) {
    waypoint::v1::SimAdvanceResponse resp;
    if (clock_->mode() != wp::util::SimClock::Mode::Controlled) {
        resp.set_error("sim clock not in controlled mode");
        return resp;
    }
    resp.set_virtual_now_mono_ns(clock_->grantAndWait(r.advance_ms()));
    return resp;
}

waypoint::v1::SimInfo SimControlServer::buildInfo() const {
    waypoint::v1::SimInfo info;
    info.set_sim_active(true);
    info.set_clock_mode(clock_->mode() == wp::util::SimClock::Mode::Controlled
                            ? "sim-controlled"
                            : "sim-realtime");
    info.set_virtual_now_mono_ns(clock_->virtualMonoNs());
    info.set_seed(model_->seed());
    info.set_override_count(model_->overrideCount());
    info.set_malformed_frames(malformed_ ? malformed_() : 0u);
    return info;
}

}  // namespace wp::sim
