#include "sim/SimControlServer.hpp"

#include <gtest/gtest.h>

#include "messages/sim.pb.h"
#include "sim/ServoModel.hpp"
#include "util/Clock.hpp"

namespace {

TEST(SimControlServer, ScenarioAppliesToModel) {
    auto model = std::make_shared<wp::sim::ServoModel>(
        std::vector<wp::sim::ServoConfig>{{.id = 7, .wheelMode = true}}, 1);
    wp::util::SimClock clock(wp::util::SimClock::Mode::Realtime);
    wp::sim::SimControlServer ctl(nullptr, "test", model, &clock);

    waypoint::v1::SimScenario s;
    s.set_servo_id(7);
    s.set_absent(true);
    ctl.handleScenario(s);
    EXPECT_FALSE(model->present(7));

    waypoint::v1::SimScenario clear;
    clear.set_clear(true);
    ctl.handleScenario(clear);
    EXPECT_TRUE(model->present(7));
}

TEST(SimControlServer, ScenarioUnknownServoIgnored) {
    auto model = std::make_shared<wp::sim::ServoModel>(
        std::vector<wp::sim::ServoConfig>{{.id = 7, .wheelMode = true}}, 1);
    wp::util::SimClock clock(wp::util::SimClock::Mode::Realtime);
    wp::sim::SimControlServer ctl(nullptr, "test", model, &clock);
    waypoint::v1::SimScenario s;
    s.set_servo_id(99);
    s.set_absent(true);
    ctl.handleScenario(s);  // must not crash; logged and ignored
    EXPECT_EQ(ctl.buildInfo().override_count(), 0u);
}

TEST(SimControlServer, AdvanceRejectedOutsideControlledMode) {
    auto model = std::make_shared<wp::sim::ServoModel>(
        std::vector<wp::sim::ServoConfig>{{.id = 7, .wheelMode = true}}, 1);
    wp::util::SimClock clock(wp::util::SimClock::Mode::Realtime);
    wp::sim::SimControlServer ctl(nullptr, "test", model, &clock);
    waypoint::v1::SimAdvanceRequest req;
    req.set_advance_ms(100);
    auto resp = ctl.handleAdvance(req);
    EXPECT_TRUE(resp.has_error());
}

TEST(SimControlServer, InfoReportsClockModeAndSeed) {
    auto model = std::make_shared<wp::sim::ServoModel>(
        std::vector<wp::sim::ServoConfig>{{.id = 7, .wheelMode = true}}, 1234);
    wp::util::SimClock clock(wp::util::SimClock::Mode::Realtime);
    wp::sim::SimControlServer ctl(nullptr, "test", model, &clock);
    auto info = ctl.buildInfo();
    EXPECT_TRUE(info.sim_active());
    EXPECT_EQ(info.clock_mode(), "sim-realtime");
    EXPECT_EQ(info.seed(), 1234u);
}
}  // namespace
