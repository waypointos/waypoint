#include <gtest/gtest.h>

#include <memory>

#include "servo/MockUart.hpp"
#include "servo/Sts3215Bus.hpp"
#include "servo/Sts3215Frame.hpp"

using namespace wp::servo;

TEST(ServoBus, PingRoundTrip) {
    auto mock = std::make_unique<MockUart>([](const std::vector<std::uint8_t>& req) {
        // Synthesize a status response: FF FF id 02 00 checksum
        std::uint8_t id = req[2];
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    EXPECT_EQ(bus.ping(0x07), 0);
}

TEST(ServoBus, ReadOperatingMode) {
    auto mock = std::make_unique<MockUart>([](const std::vector<std::uint8_t>& req) {
        // Reply: FF FF id 03 00 <mode> cs (one data byte).
        std::uint8_t id = req[2];
        std::uint8_t data = 0x01;  // wheel mode
        std::uint8_t length = 0x03;
        std::uint8_t params[] = {data};
        std::uint8_t cs = checksum(id, length, 0x00, params, 1);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, length, 0x00, data, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    auto m = bus.readOperatingMode(0x07);
    ASSERT_TRUE(m.has_value());
    EXPECT_EQ(*m, 0x01);
}

TEST(ServoBus, SetOperatingModeWritesMatchingFrame) {
    std::vector<std::uint8_t> captured;
    auto mock = std::make_unique<MockUart>([&](const std::vector<std::uint8_t>& req) {
        captured = req;
        std::uint8_t id = req[2];
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    EXPECT_EQ(bus.setOperatingMode(0x07, 1), 0);
    ASSERT_GE(captured.size(), 8u);
    EXPECT_EQ(captured[2], 0x07);
    EXPECT_EQ(captured[4], 0x03);  // WriteData
    EXPECT_EQ(captured[5], 0x21);  // MODE
    EXPECT_EQ(captured[6], 0x01);  // wheel
}

TEST(ServoBus, SetEEPROMLockWritesMatchingFrame) {
    std::vector<std::uint8_t> captured;
    auto mock = std::make_unique<MockUart>([&](const std::vector<std::uint8_t>& req) {
        captured = req;
        std::uint8_t id = req[2];
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    EXPECT_EQ(bus.setEEPROMLock(0x07, false), 0);
    ASSERT_GE(captured.size(), 8u);
    EXPECT_EQ(captured[5], 0x37);  // LOCK
    EXPECT_EQ(captured[6], 0x00);  // unlocked
}

TEST(ServoBus, ReadStateBlock) {
    auto mock = std::make_unique<MockUart>([](const std::vector<std::uint8_t>&) {
        // 15 bytes covering 0x38..0x46:
        // pos=2048(LE), speed=0, load=0, V=70(.1V), T=35, 5 padding bytes (0x40..0x44),
        // then PRESENT_CURRENT_L=0x10, PRESENT_CURRENT_H=0x00 -> currentRaw=0x0010.
        std::vector<std::uint8_t> data = {
            0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 70, 35,
            0, 0, 0, 0, 0,   // 5 padding registers 0x40..0x44
            0x10, 0x00       // PRESENT_CURRENT at 0x45 (lo), 0x46 (hi)
        };
        std::uint8_t id = 0x07;
        std::uint8_t length = static_cast<std::uint8_t>(data.size() + 2);
        std::uint8_t cs = checksum(id, length, 0x00, data.data(), data.size());
        std::vector<std::uint8_t> out = {0xFF, 0xFF, id, length, 0x00};
        out.insert(out.end(), data.begin(), data.end());
        out.push_back(cs);
        return out;
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    auto st = bus.readState(0x07);
    ASSERT_TRUE(st.has_value());
    EXPECT_EQ(st->positionRaw, 0x0800);
    EXPECT_EQ(st->voltageDeci, 70);
    EXPECT_EQ(st->temperatureC, 35);
    EXPECT_EQ(st->currentRaw, 0x0010);
}

// PRESENT_SPEED (bit 15) and PRESENT_LOAD (bit 10) are sign-magnitude; a
// reversing wheel must read as a small negative, not a huge two's-complement one.
TEST(ServoBus, ReadStateDecodesSignMagnitudeSpeedAndLoad) {
    auto mock = std::make_unique<MockUart>([](const std::vector<std::uint8_t>&) {
        // speed=0x8200 -> -512; load=0x0464 (0x400 sign | 100) -> -100.
        std::vector<std::uint8_t> data = {
            0x00, 0x08, 0x00, 0x82, 0x64, 0x04, 70, 35,
            0, 0, 0, 0, 0,
            0x10, 0x00
        };
        std::uint8_t id = 0x07;
        std::uint8_t length = static_cast<std::uint8_t>(data.size() + 2);
        std::uint8_t cs = checksum(id, length, 0x00, data.data(), data.size());
        std::vector<std::uint8_t> out = {0xFF, 0xFF, id, length, 0x00};
        out.insert(out.end(), data.begin(), data.end());
        out.push_back(cs);
        return out;
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    auto st = bus.readState(0x07);
    ASSERT_TRUE(st.has_value());
    EXPECT_EQ(st->speedRaw, -512);
    EXPECT_EQ(st->loadRaw, -100);
}

TEST(ServoBus, SetGoalPositionWritesMatchingFrame) {
    std::vector<std::uint8_t> captured;
    auto mock = std::make_unique<MockUart>([&](const std::vector<std::uint8_t>& req) {
        captured = req;
        std::uint8_t id = req[2];
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    EXPECT_EQ(bus.setGoalPosition(0x03, 0x0800), 0);
    ASSERT_GE(captured.size(), 9u);
    EXPECT_EQ(captured[5], 0x2A);  // GOAL_POSITION
    EXPECT_EQ(captured[6], 0x00);
    EXPECT_EQ(captured[7], 0x08);
}

// Safe-goal latch: enabling torque on a position-mode servo must first write
// GOAL_POSITION = PRESENT_POSITION, THEN enable torque. We assert the order of
// writes seen on the bus.
TEST(ServoBus, EnableTorqueHoldingPositionLatchesGoalThenEnables) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto mock = std::make_unique<MockUart>([&](const std::vector<std::uint8_t>& req) {
        writes.push_back(req);
        std::uint8_t id = req[2];
        // ReadData on MODE (0x21) -> reply mode=0 (position).
        if (req[4] == 0x02 && req[5] == 0x21) {
            std::uint8_t data = 0x00, len = 0x03, params[] = {data};
            std::uint8_t cs = checksum(id, len, 0x00, params, 1);
            return std::vector<std::uint8_t>{0xFF, 0xFF, id, len, 0x00, data, cs};
        }
        // ReadData on PRESENT_POSITION block (0x38) -> 15 bytes, position=0x0ABC.
        if (req[4] == 0x02 && req[5] == 0x38) {
            std::vector<std::uint8_t> d = {0xBC, 0x0A, 0,0, 0,0, 70, 35, 0,0,0,0,0, 0,0};
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
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    EXPECT_EQ(bus.enableTorqueHoldingPosition(0x03), 0);

    // Find the GOAL_POSITION write and the TORQUE_ENABLE(=1) write; goal must precede enable.
    int goalIdx = -1, enableIdx = -1;
    for (int i = 0; i < static_cast<int>(writes.size()); ++i) {
        if (writes[i][4] == 0x03 && writes[i][5] == 0x2A) goalIdx = i;
        if (writes[i][4] == 0x03 && writes[i][5] == 0x28 && writes[i][6] == 0x01) enableIdx = i;
    }
    ASSERT_GE(goalIdx, 0);
    ASSERT_GE(enableIdx, 0);
    EXPECT_LT(goalIdx, enableIdx);
    // Latched goal equals present position 0x0ABC.
    EXPECT_EQ(writes[goalIdx][6], 0xBC);
    EXPECT_EQ(writes[goalIdx][7], 0x0A);
}

// Broadcast SYNC WRITE gets no reply, so the bus must emit exactly one frame
// and never read. MockUart has no read-counter ctor, so we assert on writes.
TEST(Sts3215Bus, SyncWriteEmitsOneFrameAndDoesNotRead) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto mock = std::make_unique<MockUart>(
        [&writes](const std::vector<std::uint8_t>& req) {
            writes.push_back(req);
            return std::vector<std::uint8_t>{};  // no reply for broadcast
        });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");

    EXPECT_EQ(bus.syncWriteGoalPositions({{1, 100}, {3, 512}}), 0);
    ASSERT_EQ(writes.size(), 1u);
    EXPECT_EQ(writes[0][2], 0xFE);          // broadcast id
    EXPECT_EQ(writes[0][4], 0x83);          // SyncWrite
}

// setTuning writes EEPROM fields inside one unlock/relock cycle, then SRAM
// fields with no lock, and omits any field that is not set.
TEST(ServoBus, SetTuningEmitsLockedEepromThenSramSequence) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto mock = std::make_unique<MockUart>([&](const std::vector<std::uint8_t>& req) {
        writes.push_back(req);
        std::uint8_t id = req[2];
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");

    Sts3215Bus::Tuning t;
    t.pCoef = 8;                       // EEPROM (0x15)
    t.returnDelay = 0;                 // EEPROM (0x07)
    t.acceleration = 50;              // SRAM   (0x29)
    t.maxAcceleration = 0x0123;       // SRAM   (0x55, 2 bytes)
    // iCoef and dCoef intentionally left unset.
    EXPECT_EQ(bus.setTuning(0x03, t), 0);

    // Expected order: LOCK=0, RETURN_DELAY, COMP_P, LOCK=1, ACCELERATION, MAX_ACCELERATION.
    ASSERT_EQ(writes.size(), 6u);
    EXPECT_EQ(writes[0][5], 0x37); EXPECT_EQ(writes[0][6], 0x00);  // unlock
    EXPECT_EQ(writes[1][5], 0x07); EXPECT_EQ(writes[1][6], 0x00);  // RETURN_DELAY
    EXPECT_EQ(writes[2][5], 0x15); EXPECT_EQ(writes[2][6], 0x08);  // COMP_P
    EXPECT_EQ(writes[3][5], 0x37); EXPECT_EQ(writes[3][6], 0x01);  // relock
    EXPECT_EQ(writes[4][5], 0x29); EXPECT_EQ(writes[4][6], 50);    // ACCELERATION
    EXPECT_EQ(writes[5][5], 0x55);                                 // MAX_ACCELERATION
    EXPECT_EQ(writes[5][6], 0x23); EXPECT_EQ(writes[5][7], 0x01);  // LE word

    // Unset I/D coefficients are never written.
    for (auto& w : writes) {
        EXPECT_FALSE(w[5] == 0x16);  // COMP_D
        EXPECT_FALSE(w[5] == 0x17);  // COMP_I
    }
}

// With only SRAM fields set, setTuning must not touch the EEPROM lock at all.
TEST(ServoBus, SetTuningSramOnlySkipsLock) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto mock = std::make_unique<MockUart>([&](const std::vector<std::uint8_t>& req) {
        writes.push_back(req);
        std::uint8_t id = req[2];
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");

    Sts3215Bus::Tuning t;
    t.acceleration = 30;
    EXPECT_EQ(bus.setTuning(0x03, t), 0);
    ASSERT_EQ(writes.size(), 1u);
    EXPECT_EQ(writes[0][5], 0x29);  // ACCELERATION only
}

// In wheel mode there is no goal-position snap, so no GOAL_POSITION write.
TEST(ServoBus, EnableTorqueHoldingPositionSkipsLatchInWheelMode) {
    std::vector<std::vector<std::uint8_t>> writes;
    auto mock = std::make_unique<MockUart>([&](const std::vector<std::uint8_t>& req) {
        writes.push_back(req);
        std::uint8_t id = req[2];
        if (req[4] == 0x02 && req[5] == 0x21) {  // MODE read -> 1 (wheel)
            std::uint8_t data = 0x01, len = 0x03, params[] = {data};
            std::uint8_t cs = checksum(id, len, 0x00, params, 1);
            return std::vector<std::uint8_t>{0xFF, 0xFF, id, len, 0x00, data, cs};
        }
        std::uint8_t cs = checksum(id, 0x02, 0x00, nullptr, 0);
        return std::vector<std::uint8_t>{0xFF, 0xFF, id, 0x02, 0x00, cs};
    });
    Sts3215Bus bus(std::move(mock));
    bus.open("/dev/null");
    EXPECT_EQ(bus.enableTorqueHoldingPosition(0x03), 0);
    for (auto& w : writes) EXPECT_FALSE(w[4] == 0x03 && w[5] == 0x2A);  // no GOAL_POSITION write
}
