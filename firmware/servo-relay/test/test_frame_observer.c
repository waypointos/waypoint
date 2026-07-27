#include "unity.h"
#include "counters.h"
#include "frame_observer.h"

static void reset_all(void) {
    counters_reset();
    frame_observer_reset();
}

void setUp(void)    { reset_all(); }
void tearDown(void) {}

// Ping ID=1: FF FF 01 02 01 FB
//   length=2, instr=01 (Ping), checksum = ~(01+02+01) = ~04 = FB
static const uint8_t PING_ID1[] = { 0xFF, 0xFF, 0x01, 0x02, 0x01, 0xFB };

void test_wellformed_ping_in_one_chunk(void) {
    frame_observer_feed(FO_DIR_PI_TO_SERVO, PING_ID1, sizeof(PING_ID1));

    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(1, snap.frames_ok[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_bad_checksum[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_truncated[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(sizeof(PING_ID1), snap.bytes_total[FO_DIR_PI_TO_SERVO]);
}

void test_wellformed_ping_split_byte_by_byte(void) {
    for (size_t i = 0; i < sizeof(PING_ID1); ++i) {
        frame_observer_feed(FO_DIR_PI_TO_SERVO, &PING_ID1[i], 1);
    }
    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(1, snap.frames_ok[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_bad_checksum[FO_DIR_PI_TO_SERVO]);
}

void test_bad_checksum_counted(void) {
    uint8_t bad[] = { 0xFF, 0xFF, 0x01, 0x02, 0x01, 0x00 };  // checksum should be 0xFB
    frame_observer_feed(FO_DIR_SERVO_TO_PI, bad, sizeof(bad));
    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_ok[FO_DIR_SERVO_TO_PI]);
    TEST_ASSERT_EQUAL_UINT32(1, snap.frames_bad_checksum[FO_DIR_SERVO_TO_PI]);
}

void test_pure_noise_no_counters(void) {
    uint8_t noise[] = { 0x01, 0x02, 0x03, 0x04, 0x05 };
    frame_observer_feed(FO_DIR_PI_TO_SERVO, noise, sizeof(noise));
    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_ok[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_bad_checksum[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_truncated[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(sizeof(noise), snap.bytes_total[FO_DIR_PI_TO_SERVO]);
}

void test_two_frames_back_to_back(void) {
    // Ping ID=1 followed by Ping ID=2 (checksum = ~(02+02+01) = ~05 = FA).
    uint8_t two[] = {
        0xFF, 0xFF, 0x01, 0x02, 0x01, 0xFB,
        0xFF, 0xFF, 0x02, 0x02, 0x01, 0xFA,
    };
    frame_observer_feed(FO_DIR_PI_TO_SERVO, two, sizeof(two));
    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(2, snap.frames_ok[FO_DIR_PI_TO_SERVO]);
}

void test_length_overflow_resets_without_crash(void) {
    // length = 0xFF is well above FO_MAX_LENGTH (64).
    uint8_t bad[] = { 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0x02, 0x03 };
    frame_observer_feed(FO_DIR_PI_TO_SERVO, bad, sizeof(bad));
    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_ok[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(1, snap.frames_truncated[FO_DIR_PI_TO_SERVO]);
}

void test_noise_frame_noise_frame_resyncs_cleanly(void) {
    // Noise -> valid -> noise -> valid. Observer must not get stuck.
    uint8_t leading_noise[]  = { 0x42, 0x99, 0x00 };
    uint8_t middle_noise[]   = { 0x55, 0xAA, 0x01, 0x02 };
    // Ping ID=2: FF FF 02 02 01 FA
    uint8_t ping_id2[]       = { 0xFF, 0xFF, 0x02, 0x02, 0x01, 0xFA };

    frame_observer_feed(FO_DIR_PI_TO_SERVO, leading_noise, sizeof(leading_noise));
    frame_observer_feed(FO_DIR_PI_TO_SERVO, PING_ID1, sizeof(PING_ID1));
    frame_observer_feed(FO_DIR_PI_TO_SERVO, middle_noise, sizeof(middle_noise));
    frame_observer_feed(FO_DIR_PI_TO_SERVO, ping_id2, sizeof(ping_id2));

    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(2, snap.frames_ok[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_bad_checksum[FO_DIR_PI_TO_SERVO]);
}

void test_tick_marks_partial_frame_as_truncated(void) {
    uint8_t partial[] = { 0xFF, 0xFF, 0x01, 0x02, 0x01 };  // checksum missing
    frame_observer_feed(FO_DIR_PI_TO_SERVO, partial, sizeof(partial));

    // Inform observer current time = 6 ms after the implicit t=0 of feed.
    // The observer's last_byte_us starts at 0; we treat any tick > 5000 us
    // beyond that as a timeout.
    frame_observer_tick(6000);

    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(1, snap.frames_truncated[FO_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(1, snap.frames_resync[FO_DIR_PI_TO_SERVO]);

    // After timeout, a new frame should parse cleanly.
    frame_observer_feed(FO_DIR_PI_TO_SERVO, PING_ID1, sizeof(PING_ID1));
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(1, snap.frames_ok[FO_DIR_PI_TO_SERVO]);
}

int main(void) {
    UNITY_BEGIN();
    RUN_TEST(test_wellformed_ping_in_one_chunk);
    RUN_TEST(test_wellformed_ping_split_byte_by_byte);
    RUN_TEST(test_bad_checksum_counted);
    RUN_TEST(test_pure_noise_no_counters);
    RUN_TEST(test_two_frames_back_to_back);
    RUN_TEST(test_length_overflow_resets_without_crash);
    RUN_TEST(test_noise_frame_noise_frame_resyncs_cleanly);
    RUN_TEST(test_tick_marks_partial_frame_as_truncated);
    return UNITY_END();
}
