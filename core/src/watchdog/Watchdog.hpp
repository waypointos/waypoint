#pragma once

#include <chrono>
#include <functional>
#include <mutex>

namespace wp::watchdog {

class Watchdog {
public:
    using Clock = std::chrono::steady_clock;
    using TimePoint = Clock::time_point;

    Watchdog(std::chrono::milliseconds timeout,
             std::function<TimePoint()> now = []{ return Clock::now(); })
        : timeout_(timeout), now_(std::move(now)) {}

    // Call when a heartbeat is received. Sets the timer.
    void kick();

    // Periodically called by the main loop. Returns true if the watchdog
    // crossed the timeout boundary (the caller should drop to safe mode).
    // Returns false otherwise.
    bool tickAndCheck();

    bool isAlive() const {
        std::lock_guard<std::mutex> lk(mu_);
        return alive_;
    }

private:
    // kick() runs on the NATS reader thread; tickAndCheck()/isAlive() run on the
    // control-loop thread. mu_ guards the shared state against that data race.
    mutable std::mutex mu_;
    std::chrono::milliseconds timeout_;
    std::function<TimePoint()> now_;
    TimePoint lastKick_{};
    bool alive_ = false;
};

}  // namespace wp::watchdog
