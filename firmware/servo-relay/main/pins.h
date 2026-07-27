#pragma once

#include "driver/gpio.h"
#include "driver/uart.h"

// UART0 — Pi link (fixed by ESP32 module layout, wired to Pi GPIO 14/15
// via the Waveshare HAT's Raspberry Interface header).
#define UART_PI            UART_NUM_0
#define PIN_UART_PI_TX     GPIO_NUM_1    // U0TXD → Pi GPIO 15 (RXD0)
#define PIN_UART_PI_RX     GPIO_NUM_3    // U0RXD ← Pi GPIO 14 (TXD0)

// UART1 — servo bus via SP3485EN RS485 transceiver.
// Source: Waveshare reference firmware ServoDriverST.ino (S_TXD=19, S_RXD=18).
#define UART_SERVO         UART_NUM_1
#define PIN_UART_SERVO_TX  GPIO_NUM_19   // U1TXD → SP3485EN DI
#define PIN_UART_SERVO_RX  GPIO_NUM_18   // U1RXD ← SP3485EN RO
// Note: TXEN (RS485 DE/RE) is driven automatically by the HAT's Q1
// transistor network from U1TXD activity. No firmware GPIO required.

#define UART_BAUD          1000000
