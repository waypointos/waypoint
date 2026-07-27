#pragma once

#include <cstdint>
#include <functional>

#include "messages/common.pb.h"

namespace wp::util {

// Fills a dual capture-time stamp: wall clock, CLOCK_MONOTONIC ns since boot,
// and the kernel boot id ("" where unavailable). Call at observation time.
void fillStamp(waypoint::v1::Stamp* s);

// Override the monotonic source for capture stamps (sim virtual time).
// nullptr restores CLOCK_MONOTONIC. Set once at boot before the loop starts;
// not thread-safe against concurrent fillStamp callers mid-swap.
void setMonoNsSource(std::function<std::uint64_t()> src);

}  // namespace wp::util
