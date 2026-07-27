#include <gtest/gtest.h>

#include "mode/ModeMachine.hpp"

using namespace wp::mode;

TEST(Mode, BootsInSafe) {
    ModeMachine m;
    EXPECT_EQ(m.current(), Mode::Safe);
}

TEST(Mode, EnterManualOnlyAfterHeartbeat) {
    ModeMachine m;
    EXPECT_FALSE(m.requestSetMode(Mode::Manual));
    m.onHeartbeatRestored();
    EXPECT_TRUE(m.requestSetMode(Mode::Manual));
    EXPECT_EQ(m.current(), Mode::Manual);
}

TEST(Mode, HeartbeatLossFromManualGoesToSafe) {
    ModeMachine m;
    m.onHeartbeatRestored();
    m.requestSetMode(Mode::Manual);
    m.onHeartbeatLost();
    EXPECT_EQ(m.current(), Mode::Safe);
}

TEST(Mode, EstopBlocksTransitions) {
    ModeMachine m;
    m.onHeartbeatRestored();
    m.requestEstop();
    EXPECT_FALSE(m.requestSetMode(Mode::Manual));
    EXPECT_TRUE(m.requestRecover());
    EXPECT_EQ(m.current(), Mode::Safe);
}

TEST(Mode, RecoverWithoutHeartbeatGoesToSafe) {
    ModeMachine m;
    m.requestEstop();                 // boot heartbeat is absent
    EXPECT_EQ(m.current(), Mode::Estop);
    EXPECT_TRUE(m.requestRecover());
    EXPECT_EQ(m.current(), Mode::Safe);
}

TEST(Mode, RecoverIsNoOpWhenNotEstopped) {
    ModeMachine m;
    m.onHeartbeatRestored();
    m.requestSetMode(Mode::Manual);
    EXPECT_FALSE(m.requestRecover());
    EXPECT_EQ(m.current(), Mode::Manual);
}
