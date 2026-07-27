#include "util/Clock.hpp"

#include <algorithm>
#include <thread>

namespace wp::util {

void RealClock::sleepFor(std::chrono::milliseconds d) {
    std::this_thread::sleep_for(d);
}

SimClock::SimClock(Mode m)
    : mode_(m),
      realStart_(std::chrono::steady_clock::now()),
      vbase_(realStart_) {}

SimClock::TimePoint SimClock::now() {
    if (mode_ == Mode::Realtime) {
        return vbase_ + (std::chrono::steady_clock::now() - realStart_);
    }
    std::lock_guard<std::mutex> lk(mu_);
    return vbase_ + consumed_;
}

void SimClock::sleepFor(std::chrono::milliseconds d) {
    if (mode_ == Mode::Realtime) {
        std::this_thread::sleep_for(d);
        return;
    }
    std::unique_lock<std::mutex> lk(mu_);
    if (awaitingTickPass_) {
        // Reaching sleepFor again means the loop ran its tick pass for the
        // grant's final slice; everything the grant caused is published.
        // Only now may grantAndWait return.
        awaitingTickPass_ = false;
        cv_.notify_all();
    }
    cv_.wait(lk, [&] { return pendingMs_ > 0 || stopped_; });
    if (stopped_) return;
    auto step = std::min<std::uint64_t>(pendingMs_, static_cast<std::uint64_t>(d.count()));
    pendingMs_ -= step;
    consumed_ += std::chrono::milliseconds(step);
    if (pendingMs_ == 0) awaitingTickPass_ = true;
}

std::uint64_t SimClock::grantAndWait(std::uint64_t advanceMs) {
    std::unique_lock<std::mutex> lk(mu_);
    pendingMs_ += advanceMs;
    cv_.notify_all();
    cv_.wait(lk, [&] { return (pendingMs_ == 0 && !awaitingTickPass_) || stopped_; });
    return static_cast<std::uint64_t>(
        std::chrono::duration_cast<std::chrono::nanoseconds>(
            consumed_ + (vbase_ - realStart_)).count());
}

std::uint64_t SimClock::virtualMonoNs() const {
    if (mode_ == Mode::Realtime) {
        return static_cast<std::uint64_t>(
            std::chrono::duration_cast<std::chrono::nanoseconds>(
                std::chrono::steady_clock::now() - realStart_).count());
    }
    std::lock_guard<std::mutex> lk(mu_);
    return static_cast<std::uint64_t>(
        std::chrono::duration_cast<std::chrono::nanoseconds>(consumed_).count());
}

void SimClock::stop() {
    std::lock_guard<std::mutex> lk(mu_);
    stopped_ = true;
    cv_.notify_all();
}

}  // namespace wp::util
