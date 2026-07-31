#include "mode/ModeMachine.hpp"

namespace wp::mode {

const char* name(Mode m) {
    switch (m) {
    case Mode::Manual:     return "manual";
    case Mode::Safe:       return "safe";
    case Mode::Estop:      return "estop";
    case Mode::Autonomous: return "autonomous";
    }
    return "?";
}

bool ModeMachine::requestSetMode(Mode target) {
    // Cannot leave estop except via explicit recover.
    if (mode_ == Mode::Estop) return false;
    // Cannot enter an actuating mode without a live heartbeat.
    if ((target == Mode::Manual || target == Mode::Autonomous) && !heartbeatOk_) return false;
    transition(target);
    return true;
}

bool ModeMachine::requestEstop() {
    transition(Mode::Estop);
    return true;
}

bool ModeMachine::requestRecover() {
    if (mode_ == Mode::Estop) {
        transition(Mode::Safe);
        return true;
    }
    return false;
}

void ModeMachine::onHeartbeatLost() {
    heartbeatOk_ = false;
    if (mode_ == Mode::Manual || mode_ == Mode::Autonomous) transition(Mode::Safe);
}

void ModeMachine::onHeartbeatRestored() {
    heartbeatOk_ = true;
    // Restoration does NOT auto-resume Manual or Autonomous; that requires
    // explicit rpc.set_mode.
}

void ModeMachine::transition(Mode to) {
    if (to == mode_) return;
    auto old = mode_;
    mode_ = to;
    if (onChange_) onChange_(old, mode_);
}

}  // namespace wp::mode
