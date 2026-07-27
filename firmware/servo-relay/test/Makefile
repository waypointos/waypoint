CC      ?= cc
CFLAGS  := -std=c11 -Wall -Wextra -Werror -I../main -Iunity
UNITY   := unity/unity.c

.PHONY: check clean

check: run_counters run_frame_observer

run_counters: test_counters
	./test_counters

run_frame_observer: test_frame_observer
	./test_frame_observer

test_counters: test_counters.c ../main/counters.c $(UNITY)
	$(CC) $(CFLAGS) -o $@ $^

test_frame_observer: test_frame_observer.c ../main/frame_observer.c ../main/counters.c $(UNITY)
	$(CC) $(CFLAGS) -o $@ $^

clean:
	rm -f test_counters test_frame_observer
