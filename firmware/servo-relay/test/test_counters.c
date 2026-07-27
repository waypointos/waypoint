#include "unity.h"
#include "counters.h"

void setUp(void)    { counters_reset(); }
void tearDown(void) {}

void test_reset_zeros_all_fields(void) {
    counters_inc_frames_ok(CTR_DIR_PI_TO_SERVO);
    counters_inc_frames_bad_checksum(CTR_DIR_SERVO_TO_PI);
    counters_add_bytes(CTR_DIR_PI_TO_SERVO, 42);

    counters_reset();

    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_ok[CTR_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(0, snap.frames_bad_checksum[CTR_DIR_SERVO_TO_PI]);
    TEST_ASSERT_EQUAL_UINT32(0, snap.bytes_total[CTR_DIR_PI_TO_SERVO]);
}

void test_increments_are_per_direction(void) {
    counters_inc_frames_ok(CTR_DIR_PI_TO_SERVO);
    counters_inc_frames_ok(CTR_DIR_PI_TO_SERVO);
    counters_inc_frames_ok(CTR_DIR_SERVO_TO_PI);

    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(2, snap.frames_ok[CTR_DIR_PI_TO_SERVO]);
    TEST_ASSERT_EQUAL_UINT32(1, snap.frames_ok[CTR_DIR_SERVO_TO_PI]);
}

void test_add_bytes_accumulates(void) {
    counters_add_bytes(CTR_DIR_PI_TO_SERVO, 5);
    counters_add_bytes(CTR_DIR_PI_TO_SERVO, 7);

    struct counter_snapshot snap;
    counters_snapshot(&snap);
    TEST_ASSERT_EQUAL_UINT32(12, snap.bytes_total[CTR_DIR_PI_TO_SERVO]);
}

int main(void) {
    UNITY_BEGIN();
    RUN_TEST(test_reset_zeros_all_fields);
    RUN_TEST(test_increments_are_per_direction);
    RUN_TEST(test_add_bytes_accumulates);
    return UNITY_END();
}
