#include <gtest/gtest.h>

#include "watchdog/Watchdog.hpp"

using namespace wp::watchdog;
using namespace std::chrono_literals;

TEST(Watchdog, FiresAfterTimeoutMs) {
    Watchdog::TimePoint t{};
    Watchdog wd(500ms, [&]{ return t; });

    wd.kick();
    // 400ms later: still alive
    t += 400ms;
    EXPECT_FALSE(wd.tickAndCheck());
    EXPECT_TRUE(wd.isAlive());

    // 600ms later total: timeout
    t += 200ms;
    EXPECT_TRUE(wd.tickAndCheck());
    EXPECT_FALSE(wd.isAlive());

    // Re-kick recovers
    wd.kick();
    EXPECT_TRUE(wd.isAlive());
}
