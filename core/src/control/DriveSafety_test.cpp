#include <gtest/gtest.h>

#include "control/DriveSafety.hpp"

using namespace wp::watchdog;
using namespace std::chrono_literals;

// Drive commands stop arriving (e.g. a stalled proxy uplink) → the guard must
// fire once so the caller zeroes the target, then recover on the next command.
// The heartbeat watchdog cannot catch this: it tracks the LOCAL agent link,
// which keeps kicking during an internet stall.
TEST(DriveSafety, ZeroesAfterCommandsStop) {
    Watchdog::TimePoint t{};
    wp::control::DriveSafety safety(300ms, [&]{ return t; });

    safety.onCommand();
    t += 200ms;
    EXPECT_FALSE(safety.stale());   // fresh — hold the target

    t += 200ms;                     // 400ms since last command
    EXPECT_TRUE(safety.stale());    // crossed the boundary — caller zeroes

    t += 200ms;
    EXPECT_FALSE(safety.stale());   // already fired; edge-triggered, stays false

    safety.onCommand();             // new command
    EXPECT_FALSE(safety.stale());   // recovered
}
