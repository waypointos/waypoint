idf_component_register(
    SRCS "main.c" "relay.c" "frame_observer.c" "counters.c"
    INCLUDE_DIRS "."
    REQUIRES driver esp_timer
)
