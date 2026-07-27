#include <gtest/gtest.h>

#include "telemetry/ServoStateCache.hpp"

using namespace wp::telemetry;
using wp::servo::ServoState;

namespace {
ServoState st(std::uint16_t current, std::uint8_t volt) {
    ServoState s;
    s.currentRaw = current;
    s.voltageDeci = volt;
    return s;
}
const auto kBase = ServoStateCache::Clock::time_point{} + std::chrono::seconds(10);
}  // namespace

TEST(ServoStateCache, SumsFreshCurrentAcrossServos) {
    ServoStateCache c;
    c.update(7, st(100, 120), kBase);  // 100 * 0.0065 = 0.65 A
    c.update(2, st(200, 120), kBase);  // 200 * 0.0065 = 1.30 A
    EXPECT_NEAR(c.sumCurrentFresh(kBase, std::chrono::seconds(1)), 1.95, 1e-9);
}

TEST(ServoStateCache, SumsMagnitudeWhenDirectionBitSet) {
    // STS3215 PRESENT_CURRENT is sign-magnitude: bit 15 = direction.
    // Reverse-direction reading 0x8064 has magnitude 0x64 = 100 -> 0.65 A,
    // not (0x8064 * 6.5 mA) = ~213 A.
    ServoStateCache c;
    c.update(7, st(0x8064, 120), kBase);
    c.update(2, st(50, 120), kBase);  // 50 * 0.0065 = 0.325 A
    EXPECT_NEAR(c.sumCurrentFresh(kBase, std::chrono::seconds(1)), 0.975, 1e-9);
}

TEST(ServoStateCache, ExcludesStaleEntries) {
    ServoStateCache c;
    c.update(7, st(100, 120), kBase);
    const auto later = kBase + std::chrono::seconds(2);  // beyond the 1s window
    EXPECT_NEAR(c.sumCurrentFresh(later, std::chrono::seconds(1)), 0.0, 1e-9);
    EXPECT_FALSE(c.freshestVoltage(later, std::chrono::seconds(1)).has_value());
}

TEST(ServoStateCache, VoltageFromFreshestServo) {
    ServoStateCache c;
    c.update(7, st(0, 118), kBase);                                  // 11.8 V, older
    c.update(8, st(0, 121), kBase + std::chrono::milliseconds(10));  // 12.1 V, newest
    auto v = c.freshestVoltage(kBase + std::chrono::milliseconds(10), std::chrono::seconds(1));
    ASSERT_TRUE(v.has_value());
    EXPECT_NEAR(*v, 12.1, 1e-9);
}

TEST(ServoStateCache, EmptyHasNoReadings) {
    ServoStateCache c;
    EXPECT_FALSE(c.freshestVoltage(kBase, std::chrono::seconds(1)).has_value());
    EXPECT_NEAR(c.sumCurrentFresh(kBase, std::chrono::seconds(1)), 0.0, 1e-9);
}
