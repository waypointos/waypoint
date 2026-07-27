// SPDX-License-Identifier: Apache-2.0
//
// waypoint-bootcheck: clears /data/waypoint/bootcount after waypoint-agent
// reports healthy via a signal file at /run/waypoint/healthy.
//
// Waits up to 60s for the signal; if missing, exits non-zero (does NOT clear
// the bootcount) so the firmware boot-fail loop can detect a stuck image.

#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

static const char* HEALTHY_PATH    = "/run/waypoint/healthy";
static const char* BOOTCOUNT_PATH  = "/data/waypoint/bootcount";
static const int   TIMEOUT_S       = 60;

int main(void) {
    struct stat st;
    for (int i = 0; i < TIMEOUT_S; ++i) {
        if (stat(HEALTHY_PATH, &st) == 0) {
            if (unlink(BOOTCOUNT_PATH) != 0 && errno != ENOENT) {
                fprintf(stderr, "bootcheck: unlink %s: %s\n", BOOTCOUNT_PATH, strerror(errno));
                return 1;
            }
            fprintf(stdout, "bootcheck: cleared %s after %ds\n", BOOTCOUNT_PATH, i);
            return 0;
        }
        sleep(1);
    }
    fprintf(stderr, "bootcheck: timeout waiting for %s\n", HEALTHY_PATH);
    return 1;
}
