#pragma once

#include <functional>

namespace wp::mode {

enum class Mode { Manual, Safe, Estop };

const char* name(Mode m);

class ModeMachine {
public:
    Mode current() const { return mode_; }

    // Returns true if the requested transition was allowed.
    bool requestSetMode(Mode target);
    bool requestEstop();
    bool requestRecover();  // user clears estop/safe explicitly

    // Watchdog-driven transition: heartbeat lost → safe.
    void onHeartbeatLost();
    void onHeartbeatRestored();

    void onChange(std::function<void(Mode old, Mode now)> cb) { onChange_ = std::move(cb); }

private:
    Mode mode_ = Mode::Safe;  // safe at boot until first heartbeat
    bool heartbeatOk_ = false;
    std::function<void(Mode, Mode)> onChange_;
    void transition(Mode to);
};

}  // namespace wp::mode
