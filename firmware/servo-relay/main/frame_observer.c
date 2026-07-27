#include "frame_observer.h"

#define FO_TIMEOUT_US 5000

enum fo_state {
    FO_HUNT_FF1 = 0,
    FO_HUNT_FF2,
    FO_READ_ID,
    FO_READ_LENGTH,
    FO_READ_PAYLOAD,
};

struct fo_dir_state {
    enum fo_state state;
    uint8_t id;
    uint8_t length;
    uint8_t remaining;
    uint32_t sum;
    uint64_t last_byte_us;
};

static struct fo_dir_state s_dirs[CTR_DIR_COUNT];
static uint64_t s_now_us;   // updated by tick(); used to stamp feeds.

static void reset_dir(struct fo_dir_state *d) {
    d->state = FO_HUNT_FF1;
    d->id = 0;
    d->length = 0;
    d->remaining = 0;
    d->sum = 0;
}

void frame_observer_reset(void) {
    for (int i = 0; i < CTR_DIR_COUNT; ++i) {
        reset_dir(&s_dirs[i]);
        s_dirs[i].last_byte_us = 0;
    }
    s_now_us = 0;
}

static int bounded_dir(int dir) {
    return (dir == FO_DIR_SERVO_TO_PI) ? FO_DIR_SERVO_TO_PI : FO_DIR_PI_TO_SERVO;
}

static void feed_byte(struct fo_dir_state *d, int dir, uint8_t b) {
    switch (d->state) {
    case FO_HUNT_FF1:
        if (b == 0xFF) d->state = FO_HUNT_FF2;
        break;
    case FO_HUNT_FF2:
        d->state = (b == 0xFF) ? FO_READ_ID : FO_HUNT_FF1;
        break;
    case FO_READ_ID:
        d->id = b;
        d->sum = b;
        d->state = FO_READ_LENGTH;
        break;
    case FO_READ_LENGTH:
        if (b < 2 || b > FO_MAX_LENGTH) {
            counters_inc_frames_truncated(dir);
            reset_dir(d);
            break;
        }
        d->length = b;
        d->sum += b;
        d->remaining = b;
        d->state = FO_READ_PAYLOAD;
        break;
    case FO_READ_PAYLOAD:
        if (d->remaining > 1) {
            d->sum += b;
            d->remaining--;
        } else {
            uint8_t expected = (uint8_t)(~(d->sum & 0xFF));
            if (b == expected) counters_inc_frames_ok(dir);
            else               counters_inc_frames_bad_checksum(dir);
            reset_dir(d);
        }
        break;
    }
}

void frame_observer_feed(int dir, const uint8_t *buf, size_t n) {
    int d = bounded_dir(dir);
    counters_add_bytes(d, (uint32_t)n);
    struct fo_dir_state *st = &s_dirs[d];
    for (size_t i = 0; i < n; ++i) {
        feed_byte(st, d, buf[i]);
    }
    if (n > 0) st->last_byte_us = s_now_us;
}

void frame_observer_tick(uint64_t now_us) {
    s_now_us = now_us;
    for (int i = 0; i < CTR_DIR_COUNT; ++i) {
        struct fo_dir_state *st = &s_dirs[i];
        if (st->state != FO_HUNT_FF1 &&
            now_us - st->last_byte_us > FO_TIMEOUT_US) {
            counters_inc_frames_truncated(i);
            counters_inc_frames_resync(i);
            reset_dir(st);
        }
    }
}
