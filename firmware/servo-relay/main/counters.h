#pragma once

#include <stdint.h>

#define CTR_DIR_PI_TO_SERVO 0
#define CTR_DIR_SERVO_TO_PI 1
#define CTR_DIR_COUNT       2

struct counter_snapshot {
    uint32_t frames_ok[CTR_DIR_COUNT];
    uint32_t frames_bad_checksum[CTR_DIR_COUNT];
    uint32_t frames_truncated[CTR_DIR_COUNT];
    uint32_t frames_resync[CTR_DIR_COUNT];
    uint32_t bytes_total[CTR_DIR_COUNT];
};

void counters_reset(void);
void counters_inc_frames_ok(int dir);
void counters_inc_frames_bad_checksum(int dir);
void counters_inc_frames_truncated(int dir);
void counters_inc_frames_resync(int dir);
void counters_add_bytes(int dir, uint32_t n);
void counters_snapshot(struct counter_snapshot *out);
