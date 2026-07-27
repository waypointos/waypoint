#include "sim/ServoModel.hpp"

#include <gtest/gtest.h>

#include "servo/Sts3215Frame.hpp"

namespace {

wp::sim::ServoModel wheelServo() {
    return wp::sim::ServoModel({{.id = 7, .wheelMode = true}}, 1234);
}
wp::sim::ServoModel armServo() {
    return wp::sim::ServoModel({{.id = 1, .wheelMode = false}}, 1234);
}

// Writes GOAL_SPEED exactly as the real builder encodes it.
void writeGoalSpeedViaBuilder(wp::sim::ServoModel& m, std::uint8_t id, std::int16_t raw) {
    auto frame = wp::servo::buildSetGoalSpeed(id, raw);
    // frame: FF FF id len instr addr lo hi cs -> payload at [6], [7]
    std::uint8_t payload[2] = {frame[6], frame[7]};
    ASSERT_TRUE(m.writeReg(id, wp::servo::reg::GOAL_SPEED, payload, 2));
}

std::uint16_t readU16(wp::sim::ServoModel& m, std::uint8_t id, std::uint8_t addr) {
    auto v = m.readRegs(id, addr, 2);
    EXPECT_TRUE(v.has_value());
    return static_cast<std::uint16_t>((*v)[0] | ((*v)[1] << 8));
}

// Fidelity: wheel mode acts on GOAL_SPEED even with TORQUE_ENABLE=0
// (the dual-gate quirk; safety gating must stop the writes, not rely on torque).
TEST(ServoModel, WheelModeMovesWithTorqueDisabled) {
    auto m = wheelServo();
    std::uint8_t off = 0;
    m.writeReg(7, wp::servo::reg::TORQUE_ENABLE, &off, 1);
    writeGoalSpeedViaBuilder(m, 7, 652);  // ~1 rad/s in raw steps/s
    for (int i = 0; i < 50; ++i) m.step(0.02);  // 1 s >> tau
    auto pos1 = readU16(m, 7, wp::servo::reg::PRESENT_POSITION);
    m.step(0.02);
    auto pos2 = readU16(m, 7, wp::servo::reg::PRESENT_POSITION);
    EXPECT_NE(pos1, pos2) << "wheel must move despite torque off";
}

// Fidelity: the servo is its own velocity controller (first-order response,
// no instant jump; outer software PID oscillated on hardware).
TEST(ServoModel, VelocityIsFirstOrderNotInstant) {
    auto m = wheelServo();
    writeGoalSpeedViaBuilder(m, 7, 652);
    m.step(0.02);  // one tick: far from steady state
    auto v = m.readRegs(7, wp::servo::reg::PRESENT_SPEED, 2);
    auto raw = static_cast<std::int16_t>((*v)[0] | ((*v)[1] << 8));
    EXPECT_GT(raw, 0);
    EXPECT_LT(raw, 652 / 2) << "should not reach goal in one 20 ms tick (tau ~50 ms)";
    for (int i = 0; i < 50; ++i) m.step(0.02);
    v = m.readRegs(7, wp::servo::reg::PRESENT_SPEED, 2);
    raw = static_cast<std::int16_t>((*v)[0] | ((*v)[1] << 8));
    EXPECT_NEAR(raw, 652, 35);  // within ~5% after 1 s
}

// Fidelity: position mode moves toward GOAL_POSITION only with torque on.
TEST(ServoModel, PositionModeRequiresTorque) {
    auto m = armServo();
    std::uint8_t goal[2] = {0x00, 0x0C};  // 3072
    m.writeReg(1, wp::servo::reg::GOAL_POSITION, goal, 2);
    for (int i = 0; i < 25; ++i) m.step(0.02);
    EXPECT_EQ(readU16(m, 1, wp::servo::reg::PRESENT_POSITION), 2048) << "no torque, no motion";
    std::uint8_t on = 1;
    m.writeReg(1, wp::servo::reg::TORQUE_ENABLE, &on, 1);
    for (int i = 0; i < 500; ++i) m.step(0.02);
    EXPECT_NEAR(readU16(m, 1, wp::servo::reg::PRESENT_POSITION), 3072, 8);
}

// Fidelity: hard stops clamp motion, current rises toward stall, position
// plateaus (so100 calibration detects stops by spike OR plateau).
TEST(ServoModel, HardStopSpikesCurrentAndPlateaus) {
    auto m = armServo();
    m.setHardStops(1, 1500, 2600);
    m.setStallCurrent(1, 2.0);
    std::uint8_t on = 1;
    m.writeReg(1, wp::servo::reg::TORQUE_ENABLE, &on, 1);
    std::uint8_t goal[2] = {0x00, 0x0C};  // 3072, beyond the 2600 stop
    m.writeReg(1, wp::servo::reg::GOAL_POSITION, goal, 2);
    for (int i = 0; i < 500; ++i) m.step(0.02);
    EXPECT_EQ(readU16(m, 1, wp::servo::reg::PRESENT_POSITION), 2600) << "clamped at the stop";
    auto cur = readU16(m, 1, wp::servo::reg::PRESENT_CURRENT);
    EXPECT_NEAR(cur * 0.0065, 2.0, 0.3) << "stall current at the stop";
    auto p1 = readU16(m, 1, wp::servo::reg::PRESENT_POSITION);
    m.step(0.02);
    EXPECT_EQ(readU16(m, 1, wp::servo::reg::PRESENT_POSITION), p1) << "plateau";
}

// Fidelity: position wraps at the 0/4095 encoder seam (so100 seam-jump abort).
TEST(ServoModel, PositionWrapsAtEncoderSeam) {
    auto m = wheelServo();
    writeGoalSpeedViaBuilder(m, 7, 3000);
    // Drive forward long enough to cross 4095 from 2048.
    std::uint16_t prev = 2048;
    bool wrapped = false;
    for (int i = 0; i < 600 && !wrapped; ++i) {
        m.step(0.02);
        auto p = readU16(m, 7, wp::servo::reg::PRESENT_POSITION);
        if (p < prev && (prev - p) > 1000) wrapped = true;  // the seam snap
        prev = p;
    }
    EXPECT_TRUE(wrapped);
}

// Fidelity: current tracks velocity; bus voltage sags with summed current.
TEST(ServoModel, ElectricalModelTracksLoad) {
    wp::sim::ServoModel m({{.id = 7, .wheelMode = true}, {.id = 8, .wheelMode = true}}, 7);
    // PRESENT_VOLTAGE is a single byte; do not use the u16 helper here.
    auto vIdle = (*m.readRegs(7, wp::servo::reg::PRESENT_VOLTAGE, 1))[0];
    writeGoalSpeedViaBuilder(m, 7, 3000);
    writeGoalSpeedViaBuilder(m, 8, 3000);
    for (int i = 0; i < 100; ++i) m.step(0.02);
    auto cur = readU16(m, 7, wp::servo::reg::PRESENT_CURRENT);
    EXPECT_GT(cur * 0.0065, 0.5) << "moving current above idle";
    auto vLoaded = m.readRegs(7, wp::servo::reg::PRESENT_VOLTAGE, 1);
    EXPECT_LT((*vLoaded)[0], vIdle) << "voltage sags under load";
}

// Fidelity: temperature integrates with current and cools toward ambient.
TEST(ServoModel, ThermalIntegratesAndCools) {
    auto m = wheelServo();
    writeGoalSpeedViaBuilder(m, 7, 3000);
    for (int i = 0; i < 500; ++i) m.step(0.02);
    auto hot = m.readRegs(7, wp::servo::reg::PRESENT_TEMPERATURE, 1);
    EXPECT_GT((*hot)[0], 25);
    writeGoalSpeedViaBuilder(m, 7, 0);
    for (int i = 0; i < 3000; ++i) m.step(0.02);
    auto cooled = m.readRegs(7, wp::servo::reg::PRESENT_TEMPERATURE, 1);
    EXPECT_LT((*cooled)[0], (*hot)[0]);
}

// Fidelity: absent servos never respond (discovery / N-A telemetry paths).
TEST(ServoModel, AbsentServoDoesNotRespond) {
    auto m = wheelServo();
    m.setAbsent(7, true);
    EXPECT_FALSE(m.present(7));
    EXPECT_FALSE(m.readRegs(7, wp::servo::reg::PRESENT_POSITION, 2).has_value());
    EXPECT_FALSE(m.writeReg(7, wp::servo::reg::TORQUE_ENABLE, nullptr, 0));
}

// Determinism: same seed, same trace.
TEST(ServoModel, DeterministicUnderSeed) {
    auto run = [](std::uint64_t seed) {
        wp::sim::ServoModel m({{.id = 7, .wheelMode = true}}, seed);
        auto f = wp::servo::buildSetGoalSpeed(7, 1000);
        std::uint8_t payload[2] = {f[6], f[7]};
        m.writeReg(7, wp::servo::reg::GOAL_SPEED, payload, 2);
        std::vector<std::uint16_t> trace;
        for (int i = 0; i < 100; ++i) {
            m.step(0.02);
            auto v = m.readRegs(7, wp::servo::reg::PRESENT_VOLTAGE, 1);
            trace.push_back((*v)[0]);
        }
        return trace;
    };
    EXPECT_EQ(run(42), run(42));
    EXPECT_NE(run(42), run(43));
}

// declared() reports configured ids regardless of absence (scenario id guard).
TEST(ServoModel, DeclaredReportsConfiguredIds) {
    auto m = wheelServo();
    EXPECT_TRUE(m.declared(7));
    EXPECT_FALSE(m.declared(99));
    m.setAbsent(7, true);
    EXPECT_TRUE(m.declared(7)) << "absent but still declared";
}
}  // namespace
