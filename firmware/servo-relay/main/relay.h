#pragma once

// Spawns the two relay tasks, one per direction, pinned to opposite cores.
// Returns 0 on success. Caller must have installed UART drivers for both
// UART_PI and UART_SERVO before calling.
int relay_start(void);
