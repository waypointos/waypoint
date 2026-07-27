#include "counters.h"

#include <stdatomic.h>

static _Atomic uint32_t s_frames_ok[CTR_DIR_COUNT];
static _Atomic uint32_t s_frames_bad_checksum[CTR_DIR_COUNT];
static _Atomic uint32_t s_frames_truncated[CTR_DIR_COUNT];
static _Atomic uint32_t s_frames_resync[CTR_DIR_COUNT];
static _Atomic uint32_t s_bytes_total[CTR_DIR_COUNT];

static int bounded_dir(int dir) {
    return (dir == CTR_DIR_SERVO_TO_PI) ? CTR_DIR_SERVO_TO_PI : CTR_DIR_PI_TO_SERVO;
}

void counters_reset(void) {
    for (int i = 0; i < CTR_DIR_COUNT; ++i) {
        atomic_store(&s_frames_ok[i], 0);
        atomic_store(&s_frames_bad_checksum[i], 0);
        atomic_store(&s_frames_truncated[i], 0);
        atomic_store(&s_frames_resync[i], 0);
        atomic_store(&s_bytes_total[i], 0);
    }
}

void counters_inc_frames_ok(int dir) {
    atomic_fetch_add(&s_frames_ok[bounded_dir(dir)], 1);
}
void counters_inc_frames_bad_checksum(int dir) {
    atomic_fetch_add(&s_frames_bad_checksum[bounded_dir(dir)], 1);
}
void counters_inc_frames_truncated(int dir) {
    atomic_fetch_add(&s_frames_truncated[bounded_dir(dir)], 1);
}
void counters_inc_frames_resync(int dir) {
    atomic_fetch_add(&s_frames_resync[bounded_dir(dir)], 1);
}
void counters_add_bytes(int dir, uint32_t n) {
    atomic_fetch_add(&s_bytes_total[bounded_dir(dir)], n);
}

void counters_snapshot(struct counter_snapshot *out) {
    for (int i = 0; i < CTR_DIR_COUNT; ++i) {
        out->frames_ok[i]           = atomic_load(&s_frames_ok[i]);
        out->frames_bad_checksum[i] = atomic_load(&s_frames_bad_checksum[i]);
        out->frames_truncated[i]    = atomic_load(&s_frames_truncated[i]);
        out->frames_resync[i]       = atomic_load(&s_frames_resync[i]);
        out->bytes_total[i]         = atomic_load(&s_bytes_total[i]);
    }
}
