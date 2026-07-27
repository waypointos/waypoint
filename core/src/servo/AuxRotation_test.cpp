#include <gtest/gtest.h>

#include "servo/AuxRotation.hpp"

using wp::servo::AuxRotation;
using Ids = std::vector<std::uint8_t>;

TEST(AuxRotation, RoundRobinAdvancesAndWraps) {
    AuxRotation r({1, 2, 3}, 3);
    EXPECT_EQ(r.next(2), (Ids{1, 2}));
    EXPECT_EQ(r.next(2), (Ids{3, 1}));
    EXPECT_EQ(r.next(2), (Ids{2, 3}));
}

TEST(AuxRotation, NoDuplicatesWhenKExceedsActive) {
    AuxRotation r({5, 6}, 3);
    EXPECT_EQ(r.next(5), (Ids{5, 6}));
}

TEST(AuxRotation, DropsAfterConsecutiveFailures) {
    AuxRotation r({1, 2}, 3);
    for (int i = 0; i < 3; ++i) r.recordResult(2, false);
    EXPECT_EQ(r.active(), (Ids{1}));
    EXPECT_EQ(r.next(2), (Ids{1}));  // 2 is no longer scheduled
}

TEST(AuxRotation, SuccessResetsFailureCount) {
    AuxRotation r({1}, 3);
    r.recordResult(1, false);
    r.recordResult(1, false);
    r.recordResult(1, true);   // reset
    r.recordResult(1, false);
    r.recordResult(1, false);  // only 2 in a row since the reset
    EXPECT_EQ(r.active(), (Ids{1}));
}

TEST(AuxRotation, EmptyReturnsNothing) {
    AuxRotation r({}, 3);
    EXPECT_TRUE(r.next(3).empty());
    EXPECT_TRUE(r.active().empty());
}
