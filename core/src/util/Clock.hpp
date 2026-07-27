#pragma once

#include <chrono>
#include <condition_variable>
#include <cstdint>
#include <mutex>

namespace wp::util {

// Time seam for the control loop. RealClock on hardware; SimClock when the
// sim driver is active (virtual time, optionally harness-controlled).
class Clock {
public:
    using TimePoint = std::chrono::steady_clock::time_point;
    virtual ~Clock() = default;
    virtual TimePoint now() = 0;
    virtual void sleepFor(std::chrono::milliseconds d) = 0;
};

class RealClock : public Clock {
public:
    TimePoint now() override { return std::chrono::steady_clock::now(); }
    void sleepFor(std::chrono::milliseconds d) override;
};

class SimClock : public Clock {
public:
    enum class Mode { Realtime, Controlled };

    explicit SimClock(Mode m);

    TimePoint now() override;
    // Realtime: real sleep (virtual time tracks wall 1:1).
    // Controlled: block until granted time exists, consume up to d of it.
    void sleepFor(std::chrono::milliseconds d) override;

    // Controlled mode, called from the NATS reader thread: add a grant and
    // block until the loop has consumed all of it AND completed one more
    // full iteration (the tick pass for the final slice), so everything the
    // grant caused is published before the reply. Returns virtualMonoNs()
    // afterwards. Blocking the reader thread is intentional: it makes each
    // advance atomic (no message processing mid-grant), which is what makes
    // controlled-time tests deterministic.
    std::uint64_t grantAndWait(std::uint64_t advanceMs);

    std::uint64_t virtualMonoNs() const;
    Mode mode() const { return mode_; }

    // Unblock all sleepers/waiters for shutdown.
    void stop();

private:
    const Mode mode_;
    mutable std::mutex mu_;
    std::condition_variable cv_;
    TimePoint realStart_;
    TimePoint vbase_;
    std::chrono::nanoseconds consumed_{0};  // controlled-mode virtual elapsed
    std::uint64_t pendingMs_ = 0;
    // Set when the final slice of a grant is consumed; cleared on the loop's
    // next sleepFor entry, which proves the post-slice tick pass ran.
    bool awaitingTickPass_ = false;
    bool stopped_ = false;
};

}  // namespace wp::util
