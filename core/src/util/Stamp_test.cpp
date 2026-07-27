#include "util/Stamp.hpp"

#include <gtest/gtest.h>

#include "messages/common.pb.h"

TEST(Stamp, FillsWallAndMono) {
    waypoint::v1::Stamp s;
    wp::util::fillStamp(&s);
    EXPECT_GT(s.t().seconds(), 1700000000);  // a sane wall clock (post-2023)
    EXPECT_GT(s.mono_ns(), 0u);
}

TEST(Stamp, MonoIncreases) {
    waypoint::v1::Stamp a, b;
    wp::util::fillStamp(&a);
    wp::util::fillStamp(&b);
    EXPECT_GE(b.mono_ns(), a.mono_ns());
}
