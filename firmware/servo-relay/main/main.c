#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "driver/uart.h"
#include "esp_err.h"

#include "pins.h"
#include "relay.h"

#define UART_RX_BUF 2048
#define UART_TX_BUF 1024

static void install_uart(uart_port_t port, gpio_num_t tx, gpio_num_t rx) {
    uart_config_t cfg = {
        .baud_rate = UART_BAUD,
        .data_bits = UART_DATA_8_BITS,
        .parity    = UART_PARITY_DISABLE,
        .stop_bits = UART_STOP_BITS_1,
        .flow_ctrl = UART_HW_FLOWCTRL_DISABLE,
        .source_clk = UART_SCLK_DEFAULT,
    };
    ESP_ERROR_CHECK(uart_driver_install(port, UART_RX_BUF, UART_TX_BUF, 0, NULL, 0));
    ESP_ERROR_CHECK(uart_param_config(port, &cfg));
    ESP_ERROR_CHECK(uart_set_pin(port, tx, rx, UART_PIN_NO_CHANGE, UART_PIN_NO_CHANGE));
}

void app_main(void) {
    install_uart(UART_PI,    PIN_UART_PI_TX,    PIN_UART_PI_RX);
    install_uart(UART_SERVO, PIN_UART_SERVO_TX, PIN_UART_SERVO_RX);

    ESP_ERROR_CHECK(relay_start() == 0 ? ESP_OK : ESP_FAIL);

    for (;;) {
        vTaskDelay(pdMS_TO_TICKS(1000));
    }
}
