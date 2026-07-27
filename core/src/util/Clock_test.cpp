#include "util/Clock.hpp"

#include <atomic>
#include <thread>

#include <gtest/gtest.h>

#include "messages/common.pb.h"
#include "util/Stamp.hpp"

using namespace std::chrono_literals;

TEST(SimClock, RealtimeAdvancesWithWall) {
    wp::util::SimClock c(wp::util::SimClock::Mode::Realtime);
    auto a = c.now();
    c.sleepFor(10ms);
    auto b = c.now();
    EXPECT_GE(b - a, 10ms);
}

TEST(SimClock, ControlledOnlyAdvancesOnGrant) {
    wp::util::SimClock c(wp::util::SimClock::Mode::Controlled);
    auto start = c.now();

    // Consume grants in 5 ms slices like the control loop: the iteration
    // AFTER the final slice (the tick pass) is what releases grantAndWait.
    std::atomic<bool> done{false};
    std::thread loop([&] {
        while (!done.load()) c.sleepFor(5ms);
    });
    auto after = c.grantAndWait(100);  // returns only after the tick pass
    EXPECT_EQ(c.now() - start, 100ms);
    EXPECT_EQ(after, c.virtualMonoNs());
    done.store(true);
    c.stop();
    loop.join();
}

// The grant barrier: by the time grantAndWait returns, the loop has finished
// the tick pass following the final consumed slice, so every message that
// grant caused has been published before the rpc.sim_advance reply.
TEST(SimClock, GrantWaitsForTickPassAfterFinalSlice) {
    wp::util::SimClock c(wp::util::SimClock::Mode::Controlled);
    std::atomic<int> tickPasses{0};
    std::atomic<bool> done{false};
    std::thread loop([&] {
        while (!done.load()) {
            c.sleepFor(5ms);
            tickPasses.fetch_add(1);  // the loop's "process ticks" phase
        }
    });
    c.grantAndWait(15);  // 3 slices
    EXPECT_GE(tickPasses.load(), 3) << "reply must wait for the final tick pass";
    done.store(true);
    c.stop();
    loop.join();
}

TEST(SimClock, StopUnblocksSleepers) {
    wp::util::SimClock c(wp::util::SimClock::Mode::Controlled);
    std::atomic<bool> woke{false};
    std::thread t([&] { c.sleepFor(5ms); woke = true; });
    std::this_thread::sleep_for(20ms);
    c.stop();
    t.join();
    EXPECT_TRUE(woke);
}

TEST(StampOverride, MonoSourceFollowsInjectedClock) {
    wp::util::setMonoNsSource([] { return std::uint64_t{42}; });
    waypoint::v1::Stamp s;
    wp::util::fillStamp(&s);
    EXPECT_EQ(s.mono_ns(), 42u);
    wp::util::setMonoNsSource(nullptr);  // restore CLOCK_MONOTONIC for other tests
}
