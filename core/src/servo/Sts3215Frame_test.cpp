#include <gtest/gtest.h>

#include "servo/Sts3215Frame.hpp"

using namespace wp::servo;

TEST(Checksum, KnownPingFrame) {
    // PING servo id=1 ⇒ FF FF 01 02 01 FB
    // length = 0x02 (no params, instr + checksum = 2)
    // checksum = ~(1 + 2 + 1) = ~0x04 & 0xFF = 0xFB
    std::uint8_t cs = checksum(0x01, 0x02, 0x01, nullptr, 0);
    EXPECT_EQ(cs, 0xFB);
}

TEST(Encode, PingFrameMatches) {
    auto bytes = buildPing(0x01);
    std::vector<std::uint8_t> want = {0xFF, 0xFF, 0x01, 0x02, 0x01, 0xFB};
    EXPECT_EQ(bytes, want);
}

TEST(Encode_SetGoalSpeed, MatchesDatasheet) {
    // ID=7, GOAL_SPEED=0x2E, value=+512 (forward, low-byte first).
    // 512 = 0x0200
    auto bytes = buildSetGoalSpeed(0x07, 512);
    ASSERT_GE(bytes.size(), 9u);
    EXPECT_EQ(bytes[0], 0xFF);
    EXPECT_EQ(bytes[1], 0xFF);
    EXPECT_EQ(bytes[2], 0x07);              // id
    // length = 2 params + instr + checksum = 5
    EXPECT_EQ(bytes[3], 0x05);
    EXPECT_EQ(bytes[4], 0x03);              // WriteData
    EXPECT_EQ(bytes[5], 0x2E);              // GOAL_SPEED
    EXPECT_EQ(bytes[6], 0x00);              // low byte
    EXPECT_EQ(bytes[7], 0x02);              // high byte
    // checksum verifies
    const std::uint8_t params[] = {0x2E, 0x00, 0x02};
    std::uint8_t cs = checksum(0x07, 0x05, 0x03, params, 3);
    EXPECT_EQ(bytes[8], cs);
}

// Reverse speeds are sign-magnitude on the wire (bit 15 = direction), not
// two's complement: -512 must encode as 0x8200, never 0xFE00.
TEST(Encode_SetGoalSpeed, NegativeIsSignMagnitude) {
    auto bytes = buildSetGoalSpeed(0x07, -512);
    ASSERT_GE(bytes.size(), 9u);
    EXPECT_EQ(bytes[6], 0x00);              // low byte: magnitude 512
    EXPECT_EQ(bytes[7], 0x82);              // high byte: 0x80 direction | 0x02
}

TEST(SignMagnitude, RoundTripsSpeedAndLoad) {
    for (std::int16_t v : {std::int16_t{0}, std::int16_t{1}, std::int16_t{-1},
                           std::int16_t{652}, std::int16_t{-652}, std::int16_t{32767},
                           std::int16_t{-32767}}) {
        EXPECT_EQ(decodeSignMagnitude(encodeSignMagnitude(v, 15), 15), v);
    }
    for (std::int16_t v : {std::int16_t{0}, std::int16_t{100}, std::int16_t{-100},
                           std::int16_t{1023}, std::int16_t{-1023}}) {
        EXPECT_EQ(decodeSignMagnitude(encodeSignMagnitude(v, 10), 10), v);
    }
}

TEST(Encode_SetMode, WheelModeFrame) {
    // ID=7, MODE=0x21, value=1 (wheel).
    auto bytes = buildSetMode(0x07, 1);
    ASSERT_EQ(bytes.size(), 8u);
    EXPECT_EQ(bytes[0], 0xFF);
    EXPECT_EQ(bytes[1], 0xFF);
    EXPECT_EQ(bytes[2], 0x07);              // id
    EXPECT_EQ(bytes[3], 0x04);              // length = 2 params + instr + checksum
    EXPECT_EQ(bytes[4], 0x03);              // WriteData
    EXPECT_EQ(bytes[5], 0x21);              // MODE
    EXPECT_EQ(bytes[6], 0x01);              // wheel mode
    const std::uint8_t params[] = {0x21, 0x01};
    EXPECT_EQ(bytes[7], checksum(0x07, 0x04, 0x03, params, 2));
}

TEST(Encode_SetEEPROMLock, UnlockFrame) {
    auto bytes = buildSetEEPROMLock(0x07, false);
    ASSERT_EQ(bytes.size(), 8u);
    EXPECT_EQ(bytes[4], 0x03);              // WriteData
    EXPECT_EQ(bytes[5], 0x37);              // LOCK
    EXPECT_EQ(bytes[6], 0x00);              // unlocked
}

TEST(Encode_ReadMode, OneByteRead) {
    auto bytes = buildReadMode(0x07);
    ASSERT_EQ(bytes.size(), 8u);
    EXPECT_EQ(bytes[4], 0x02);              // ReadData
    EXPECT_EQ(bytes[5], 0x21);              // MODE
    EXPECT_EQ(bytes[6], 0x01);              // length = 1 byte
}

TEST(Decode, PingResponse) {
    // FF FF <id> 02 <err> <checksum>
    std::vector<std::uint8_t> buf = {0xFF, 0xFF, 0x01, 0x02, 0x00, 0xFC};
    auto r = decode(buf.data(), buf.size());
    ASSERT_TRUE(r.has_value());
    EXPECT_EQ(r->frame.id, 0x01);
    EXPECT_EQ(r->frame.params.size(), 1u);
    EXPECT_EQ(r->frame.params[0], 0x00);
    EXPECT_EQ(r->consumed, 6u);
}

TEST(Decode, IncompleteReturnsNullopt) {
    std::vector<std::uint8_t> buf = {0xFF, 0xFF, 0x01};
    EXPECT_FALSE(decode(buf.data(), buf.size()).has_value());
}

TEST(Decode, BadChecksumReturnsNullopt) {
    std::vector<std::uint8_t> buf = {0xFF, 0xFF, 0x01, 0x02, 0x00, 0xFF};
    EXPECT_FALSE(decode(buf.data(), buf.size()).has_value());
}

TEST(ServoFrame, SetGoalPositionLayout) {
    auto f = wp::servo::buildSetGoalPosition(0x03, 0x0800);  // 2048
    // FF FF id len WriteData reg posL posH cs
    ASSERT_EQ(f.size(), 9u);
    EXPECT_EQ(f[2], 0x03);
    EXPECT_EQ(f[4], 0x03);  // WriteData
    EXPECT_EQ(f[5], 0x2A);  // GOAL_POSITION
    EXPECT_EQ(f[6], 0x00);  // low
    EXPECT_EQ(f[7], 0x08);  // high
}

TEST(ServoFrame, SetTorqueLimitLayout) {
    auto f = wp::servo::buildSetTorqueLimit(0x03, 500);
    ASSERT_EQ(f.size(), 9u);
    EXPECT_EQ(f[5], 0x30);  // TORQUE_LIMIT
    EXPECT_EQ(f[6], static_cast<std::uint8_t>(500 & 0xFF));
    EXPECT_EQ(f[7], static_cast<std::uint8_t>((500 >> 8) & 0xFF));
}

TEST(ServoFrame, SetAngleLimitsContiguousLayout) {
    auto f = wp::servo::buildSetAngleLimits(0x03, 0x0040, 0x0FC0);
    // Writes 4 contiguous bytes at MIN_ANGLE_LIMIT (0x09): minL minH maxL maxH
    ASSERT_EQ(f.size(), 11u);
    EXPECT_EQ(f[5], 0x09);  // MIN_ANGLE_LIMIT
    EXPECT_EQ(f[6], 0x40);  // minL
    EXPECT_EQ(f[7], 0x00);  // minH
    EXPECT_EQ(f[8], 0xC0);  // maxL
    EXPECT_EQ(f[9], 0x0F);  // maxH
}

TEST(Encode_WriteByte, AccelerationFrame) {
    // ID=3, ACCELERATION=0x29, value=50.
    auto bytes = buildWriteByte(0x03, reg::ACCELERATION, 50);
    ASSERT_EQ(bytes.size(), 8u);
    EXPECT_EQ(bytes[0], 0xFF);
    EXPECT_EQ(bytes[1], 0xFF);
    EXPECT_EQ(bytes[2], 0x03);              // id
    EXPECT_EQ(bytes[3], 0x04);              // length = 2 params + instr + checksum
    EXPECT_EQ(bytes[4], 0x03);             // WriteData
    EXPECT_EQ(bytes[5], 0x29);              // ACCELERATION reg
    EXPECT_EQ(bytes[6], 50);               // value
    const std::uint8_t params[] = {0x29, 50};
    EXPECT_EQ(bytes[7], checksum(0x03, 0x04, 0x03, params, 2));
}

TEST(Encode_WriteWord, MaxAccelerationLittleEndian) {
    // ID=3, MAX_ACCELERATION=0x55, value=0x0123 -> low byte 0x23, high byte 0x01.
    auto bytes = buildWriteWord(0x03, reg::MAX_ACCELERATION, 0x0123);
    ASSERT_EQ(bytes.size(), 9u);
    EXPECT_EQ(bytes[2], 0x03);              // id
    EXPECT_EQ(bytes[3], 0x05);             // length = 3 params + instr + checksum
    EXPECT_EQ(bytes[4], 0x03);             // WriteData
    EXPECT_EQ(bytes[5], 0x55);             // MAX_ACCELERATION reg
    EXPECT_EQ(bytes[6], 0x23);             // low byte
    EXPECT_EQ(bytes[7], 0x01);             // high byte
    const std::uint8_t params[] = {0x55, 0x23, 0x01};
    EXPECT_EQ(bytes[8], checksum(0x03, 0x05, 0x03, params, 3));
}

TEST(Sts3215Frame, SyncWriteGoalPositionsTwoServos) {
    // servo 1 -> 0x0064 (100), servo 3 -> 0x0200 (512)
    std::vector<std::pair<std::uint8_t, std::uint16_t>> goals = {{1, 100}, {3, 512}};
    auto f = buildSyncWriteGoalPositions(goals);

    // FF FF FE LEN 83 2A 02  01 64 00  03 00 02  CHK
    ASSERT_EQ(f.size(), 7u + 3u * 2u + 1u);
    EXPECT_EQ(f[0], 0xFF); EXPECT_EQ(f[1], 0xFF);
    EXPECT_EQ(f[2], 0xFE);                 // broadcast id
    EXPECT_EQ(f[3], (2 + 1) * 2 + 4);      // LEN = 10
    EXPECT_EQ(f[4], 0x83);                 // SyncWrite
    EXPECT_EQ(f[5], reg::GOAL_POSITION);   // 0x2A
    EXPECT_EQ(f[6], 0x02);                 // bytes per servo
    EXPECT_EQ(f[7], 0x01); EXPECT_EQ(f[8], 0x64); EXPECT_EQ(f[9], 0x00);
    EXPECT_EQ(f[10], 0x03); EXPECT_EQ(f[11], 0x00); EXPECT_EQ(f[12], 0x02);

    // checksum over [FE, LEN, 83, payload...]
    std::vector<std::uint8_t> params(f.begin() + 5, f.end() - 1);
    EXPECT_EQ(f.back(), checksum(0xFE, f[3], 0x83, params.data(), params.size()));
}
