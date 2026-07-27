#include "relay.h"

#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "driver/uart.h"
#include "esp_timer.h"

#include "pins.h"
#include "frame_observer.h"

#define RELAY_BUF_SIZE 128
#define RELAY_READ_TICKS pdMS_TO_TICKS(2)

static void pi_to_servo_task(void *arg) {
    (void)arg;
    uint8_t buf[RELAY_BUF_SIZE];
    for (;;) {
        int n = uart_read_bytes(UART_PI, buf, sizeof(buf), RELAY_READ_TICKS);
        frame_observer_tick(esp_timer_get_time());
        if (n > 0) {
            frame_observer_feed(FO_DIR_PI_TO_SERVO, buf, (size_t)n);
            uart_write_bytes(UART_SERVO, (const char *)buf, (size_t)n);
        }
    }
}

static void servo_to_pi_task(void *arg) {
    (void)arg;
    uint8_t buf[RELAY_BUF_SIZE];
    for (;;) {
        int n = uart_read_bytes(UART_SERVO, buf, sizeof(buf), RELAY_READ_TICKS);
        frame_observer_tick(esp_timer_get_time());
        if (n > 0) {
            frame_observer_feed(FO_DIR_SERVO_TO_PI, buf, (size_t)n);
            uart_write_bytes(UART_PI, (const char *)buf, (size_t)n);
        }
    }
}

int relay_start(void) {
    BaseType_t r1 = xTaskCreatePinnedToCore(pi_to_servo_task, "relay_p2s",
                                            4096, NULL, 10, NULL, 0);
    BaseType_t r2 = xTaskCreatePinnedToCore(servo_to_pi_task, "relay_s2p",
                                            4096, NULL, 10, NULL, 1);
    return (r1 == pdPASS && r2 == pdPASS) ? 0 : -1;
}
