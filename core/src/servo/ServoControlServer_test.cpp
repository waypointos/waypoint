#include <gtest/gtest.h>

#include <memory>
#include <optional>
#include <vector>

#include "messages/servo.pb.h"
#include "servo/MockUart.hpp"
#include "servo/ServoControlServer.hpp"
#include "servo/Sts3215Bus.hpp"
#include "servo/Sts3215Frame.hpp"

using namespace wp::servo;

namespace {
// A bus whose MockUart records every request frame and answers generically.
std::unique_ptr<Sts3215Bus> makeRecordingBus(std::vector<std::vector<std::uint8_t>>* writes) {
    auto mock = std::make_unique<MockUart>([writes](const std::vector<std::uint8_t>& req) {
        writes->push_back(req);
        std::uint8_t id = req[2];
        if (req[4] == 0x02 && req[5] == 0x21) {  // MODE read -> position
            std::uint8_t data = 0x00, len = 0x03, params[] = {data};
            std::uint8_t cs = checksum(id, len, 0x00, params, 1);
            return std::vector<std::uint8_t>{0xFF, 0xFF, id, len, 0x00, data, cs};
        }
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    auto bus = std::make_unique<Sts3215Bus>(std::move(mock));
    bus->open("/dev/null");
    return bus;
}
bool hasWrite(const std::vector<std::vector<std::uint8_t>>& w, std::uint8_t instr, std::uint8_t reg) {
    for (auto& f : w) if (f.size() > 5 && f[4] == instr && f[5] == reg) return true;
    return false;
}
// Little-endian payload word of the first WRITE frame to `reg`.
std::optional<std::uint16_t> writtenWord(const std::vector<std::vector<std::uint8_t>>& w,
                                         std::uint8_t reg) {
    for (auto& f : w)
        if (f.size() > 7 && f[4] == 0x03 && f[5] == reg)
            return static_cast<std::uint16_t>(f[6] | (f[7] << 8));
    return std::nullopt;
}
}  // namespace

TEST(ServoControlServer, RejectsDriveWheelIds) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov",
                           std::vector<std::uint8_t>{7, 8, 9, 10});
    waypoint::v1::ServoControl c;
    c.set_servo_id(7);                 // back-left drive wheel
    c.set_set_goal_position(1000);
    srv.handleControl(c);
    EXPECT_FALSE(hasWrite(writes, 0x03, 0x2A));  // no GOAL_POSITION write happened
}

TEST(ServoControlServer, RejectsWhenEstopped) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return true; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(3);
    c.set_set_goal_position(1000);
    srv.handleControl(c);
    EXPECT_FALSE(hasWrite(writes, 0x03, 0x2A));
}

TEST(ServoControlServer, GoalPositionReachesBusForArmServo) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(3);
    c.set_set_goal_position(0x0800);
    srv.handleControl(c);
    EXPECT_TRUE(hasWrite(writes, 0x03, 0x2A));  // GOAL_POSITION written
}

TEST(ServoControlServer, TorqueEnableUsesSafeGoalLatch) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(3);
    c.set_set_torque_enable(true);
    srv.handleControl(c);
    // Latch path read MODE (0x21) and wrote GOAL_POSITION (0x2A) before TORQUE_ENABLE.
    EXPECT_TRUE(hasWrite(writes, 0x02, 0x21));
    EXPECT_TRUE(hasWrite(writes, 0x03, 0x28));  // TORQUE_ENABLE
}

TEST(ServoControlServer, SetTuningReachesBus) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(3);
    auto* t = c.mutable_set_tuning();
    t->set_p_coefficient(8);       // EEPROM (0x15)
    t->set_acceleration(50);       // SRAM   (0x29)
    srv.handleControl(c);
    EXPECT_TRUE(hasWrite(writes, 0x03, 0x15));   // COMP_P written
    EXPECT_TRUE(hasWrite(writes, 0x03, 0x29));   // ACCELERATION written
    // Unset I coefficient is never written.
    EXPECT_FALSE(hasWrite(writes, 0x03, 0x17));
}

TEST(ServoControlServer, SetTuningRejectedWhenEstopped) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return true; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(3);
    c.mutable_set_tuning()->set_p_coefficient(8);
    srv.handleControl(c);
    EXPECT_FALSE(hasWrite(writes, 0x03, 0x15));
}

TEST(ServoControlServer, GoalVelocityReachesBus) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov",
                           std::vector<std::uint8_t>{7, 8, 9, 10});
    waypoint::v1::ServoControl c;
    c.set_servo_id(11);                 // module-owned drill servo
    c.set_set_goal_velocity(1000);
    srv.handleControl(c);
    ASSERT_TRUE(hasWrite(writes, 0x03, 0x2E));  // GOAL_SPEED written
    EXPECT_EQ(writtenWord(writes, 0x2E), std::optional<std::uint16_t>(1000));
}

TEST(ServoControlServer, GoalVelocityNegativeEncodesSignMagnitude) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(11);
    c.set_set_goal_velocity(-1000);
    srv.handleControl(c);
    EXPECT_EQ(writtenWord(writes, 0x2E), std::optional<std::uint16_t>(1000u | 0x8000u));
}

TEST(ServoControlServer, GoalVelocityClampsOutOfRange) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(11);
    c.set_set_goal_velocity(70000);
    srv.handleControl(c);
    EXPECT_EQ(writtenWord(writes, 0x2E), std::optional<std::uint16_t>(32767));

    writes.clear();
    c.set_set_goal_velocity(-70000);
    srv.handleControl(c);
    EXPECT_EQ(writtenWord(writes, 0x2E), std::optional<std::uint16_t>(32767u | 0x8000u));
}

TEST(ServoControlServer, GoalVelocityRejectsDeniedId) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov",
                           std::vector<std::uint8_t>{7, 8, 9, 10});
    waypoint::v1::ServoControl c;
    c.set_servo_id(8);                 // back-right drive wheel
    c.set_set_goal_velocity(1000);
    srv.handleControl(c);
    EXPECT_FALSE(hasWrite(writes, 0x03, 0x2E));
}

TEST(ServoControlServer, GoalVelocityRejectedWhenEstopped) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return true; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(11);
    c.set_set_goal_velocity(1000);
    srv.handleControl(c);
    EXPECT_FALSE(hasWrite(writes, 0x03, 0x2E));
}

namespace {
// Bus whose readState reports a fixed current; records writes for trip assertion.
std::unique_ptr<Sts3215Bus> makeCurrentBus(std::uint16_t currentRaw,
                                           std::vector<std::vector<std::uint8_t>>* writes) {
    auto mock = std::make_unique<MockUart>([currentRaw, writes](const std::vector<std::uint8_t>& req) {
        if (writes) writes->push_back(req);
        std::uint8_t id = req[2];
        if (req[4] == 0x02 && req[5] == 0x38) {  // read state block
            std::vector<std::uint8_t> d = {0x00, 0x08, 0,0, 0,0, 70, 35, 0,0,0,0,0,
                                           static_cast<std::uint8_t>(currentRaw & 0xFF),
                                           static_cast<std::uint8_t>((currentRaw >> 8) & 0xFF)};
            std::uint8_t len = static_cast<std::uint8_t>(d.size() + 2);
            std::uint8_t cs = checksum(id, len, 0x00, d.data(), d.size());
            std::vector<std::uint8_t> out = {0xFF, 0xFF, id, len, 0x00};
            out.insert(out.end(), d.begin(), d.end());
            out.push_back(cs);
            return out;
        }
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    auto bus = std::make_unique<Sts3215Bus>(std::move(mock));
    bus->open("/dev/null");
    return bus;
}
}  // namespace

TEST(ServoControlServer, SyncWriteFiltersDriveWheels) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov",
                           std::vector<std::uint8_t>{7, 8, 9, 10});

    waypoint::v1::ServoSyncWrite sw;
    auto* g1 = sw.add_goals(); g1->set_servo_id(3); g1->set_goal_position(2048);
    auto* g2 = sw.add_goals(); g2->set_servo_id(7); g2->set_goal_position(1000); // drive wheel
    srv.handleSync(sw);

    // one SYNC WRITE frame emitted, and it must not carry servo 7.
    ASSERT_EQ(writes.size(), 1u);
    EXPECT_EQ(writes[0][4], 0x83);
    // body is [FF FF FE LEN 83 2A 02  (id d0 d1)...]; scan ids at offsets 7,10,...
    bool sawSeven = false;
    for (std::size_t i = 7; i + 2 < writes[0].size(); i += 3) if (writes[0][i] == 7) sawSeven = true;
    EXPECT_FALSE(sawSeven);
}

TEST(ServoControlServer, SyncWriteRejectedWhenEstopped) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeRecordingBus(&writes);
    ServoControlServer srv(nullptr, bus.get(), [] { return true; }, "rov", {});
    waypoint::v1::ServoSyncWrite sw;
    auto* g = sw.add_goals(); g->set_servo_id(3); g->set_goal_position(2048);
    srv.handleSync(sw);
    EXPECT_TRUE(writes.empty());
}

TEST(ServoControlServer, ReadReturnsRawState) {
    auto bus = makeCurrentBus(0x0010, nullptr);
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov", {});
    auto st = srv.handleRead(3);
    EXPECT_TRUE(st.ok());
    EXPECT_EQ(st.servo_id(), 3u);
    EXPECT_EQ(st.position_raw(), 0x0800u);
    EXPECT_EQ(st.current_raw(), 0x0010u);
}

TEST(ServoControlServer, OvercurrentTripsTorqueOff) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeCurrentBus(0x0200, &writes);  // 512
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(3);
    c.set_set_overcurrent_limit(256);  // ceiling below the 512 reading
    srv.handleControl(c);
    auto st = srv.handleRead(3);
    EXPECT_TRUE(st.overcurrent_tripped());
    // A TORQUE_ENABLE=0 frame was written.
    bool disabled = false;
    for (auto& f : writes) if (f[4] == 0x03 && f[5] == 0x28 && f[6] == 0x00) disabled = true;
    EXPECT_TRUE(disabled);
}

TEST(ServoControlServer, NoTripWhenUnderCeiling) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto bus = makeCurrentBus(0x0080, &writes);  // 128
    ServoControlServer srv(nullptr, bus.get(), [] { return false; }, "rov", {});
    waypoint::v1::ServoControl c;
    c.set_servo_id(3);
    c.set_set_overcurrent_limit(256);
    srv.handleControl(c);
    auto st = srv.handleRead(3);
    EXPECT_FALSE(st.overcurrent_tripped());
}
