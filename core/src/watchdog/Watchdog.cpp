#include "watchdog/Watchdog.hpp"

namespace wp::watchdog {

void Watchdog::kick() {
    std::lock_guard<std::mutex> lk(mu_);
    lastKick_ = now_();
    alive_ = true;
}

bool Watchdog::tickAndCheck() {
    std::lock_guard<std::mutex> lk(mu_);
    if (!alive_) return false;
    auto since = now_() - lastKick_;
    if (since > timeout_) {
        alive_ = false;
        return true;  // crossed boundary
    }
    return false;
}

}  // namespace wp::watchdog
