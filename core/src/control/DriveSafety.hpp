#pragma once

#include <chrono>
#include <functional>

#include "watchdog/Watchdog.hpp"

namespace wp::control {

// Zeroes the drive target when DriveCommands stop arriving, independent of the
// heartbeat watchdog. Reuses Watchdog: kick() on each command, tickAndCheck()
// fires once when the stream goes stale.
class DriveSafety {
public:
    explicit DriveSafety(
        std::chrono::milliseconds timeout,
        std::function<watchdog::Watchdog::TimePoint()> now =
            [] { return watchdog::Watchdog::Clock::now(); })
        : wd_(timeout, std::move(now)) {}

    // Call on each received DriveCommand.
    void onCommand() { wd_.kick(); }

    // Call each control tick. True when the command stream just went stale and
    // the caller must zero the drive target.
    bool stale() { return wd_.tickAndCheck(); }

private:
    watchdog::Watchdog wd_;
};

}  // namespace wp::control
