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

TEST(Mode, EnterAutonomousOnlyAfterHeartbeat) {
    ModeMachine m;
    EXPECT_FALSE(m.requestSetMode(Mode::Autonomous));
    m.onHeartbeatRestored();
    EXPECT_TRUE(m.requestSetMode(Mode::Autonomous));
    EXPECT_EQ(m.current(), Mode::Autonomous);
}

TEST(Mode, ManualAndAutonomousSwitchDirectly) {
    ModeMachine m;
    m.onHeartbeatRestored();
    EXPECT_TRUE(m.requestSetMode(Mode::Manual));
    EXPECT_TRUE(m.requestSetMode(Mode::Autonomous));
    EXPECT_EQ(m.current(), Mode::Autonomous);
    EXPECT_TRUE(m.requestSetMode(Mode::Manual));
    EXPECT_EQ(m.current(), Mode::Manual);
}

TEST(Mode, HeartbeatLossFromAutonomousGoesToSafe) {
    ModeMachine m;
    m.onHeartbeatRestored();
    m.requestSetMode(Mode::Autonomous);
    m.onHeartbeatLost();
    EXPECT_EQ(m.current(), Mode::Safe);
}

TEST(Mode, EstopFromAutonomousRecoversToSafeOnly) {
    ModeMachine m;
    m.onHeartbeatRestored();
    m.requestSetMode(Mode::Autonomous);
    m.requestEstop();
    EXPECT_EQ(m.current(), Mode::Estop);
    EXPECT_FALSE(m.requestSetMode(Mode::Autonomous));
    EXPECT_TRUE(m.requestRecover());
    EXPECT_EQ(m.current(), Mode::Safe);
}
