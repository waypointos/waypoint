#pragma once

#include <stdint.h>
#include <stddef.h>

#include "counters.h"

#define FO_DIR_PI_TO_SERVO CTR_DIR_PI_TO_SERVO
#define FO_DIR_SERVO_TO_PI CTR_DIR_SERVO_TO_PI

// Maximum plausible frame length byte. STS3215 read-state-block is the
// largest we expect: length = 14 + 2 = 16. We allow up to 64 to leave
// headroom; anything beyond that is treated as corrupt.
#define FO_MAX_LENGTH 64

// Feed bytes that just arrived in `dir`. Always counted in bytes_total.
// State machine is byte-driven; safe to split a frame across calls.
void frame_observer_feed(int dir, const uint8_t *buf, size_t n);

// Inform observer of current monotonic time (microseconds). If a partial
// frame has been buffered and the last byte arrived > 5 ms ago, declares
// the partial frame as truncated, increments frames_resync, and resets
// state to hunting for `0xFF 0xFF`. Idempotent; call as often as desired.
void frame_observer_tick(uint64_t now_us);

// Reset all internal state (does NOT reset counters). For tests.
void frame_observer_reset(void);
