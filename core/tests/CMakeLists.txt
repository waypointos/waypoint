add_executable(wp_core_tests
    ${CMAKE_SOURCE_DIR}/src/nats/Frame_test.cpp
    ${CMAKE_SOURCE_DIR}/src/nats/Connection_test.cpp
    ${CMAKE_SOURCE_DIR}/src/servo/Sts3215Frame_test.cpp
    ${CMAKE_SOURCE_DIR}/src/servo/Sts3215Bus_test.cpp
    ${CMAKE_SOURCE_DIR}/src/servo/AuxRotation_test.cpp
    ${CMAKE_SOURCE_DIR}/src/servo/Discovery_test.cpp
    ${CMAKE_SOURCE_DIR}/src/servo/ServoControlServer_test.cpp
    ${CMAKE_SOURCE_DIR}/src/telemetry/ServoStateCache_test.cpp
    ${CMAKE_SOURCE_DIR}/src/telemetry/PlatformPublisher_test.cpp
    ${CMAKE_SOURCE_DIR}/src/control/DiffDriveKinematics_test.cpp
    ${CMAKE_SOURCE_DIR}/src/control/DriveController_test.cpp
    ${CMAKE_SOURCE_DIR}/src/control/DriveSafety_test.cpp
    ${CMAKE_SOURCE_DIR}/src/mode/ModeMachine_test.cpp
    ${CMAKE_SOURCE_DIR}/src/watchdog/Watchdog_test.cpp
    ${CMAKE_SOURCE_DIR}/src/platform/Descriptor_test.cpp
    ${CMAKE_SOURCE_DIR}/src/joint/Sts3215Driver_test.cpp
    ${CMAKE_SOURCE_DIR}/src/util/Stamp_test.cpp
    ${CMAKE_SOURCE_DIR}/src/util/Clock_test.cpp
    ${CMAKE_SOURCE_DIR}/src/sim/ServoModel_test.cpp
    ${CMAKE_SOURCE_DIR}/src/sim/SimUart_test.cpp
    ${CMAKE_SOURCE_DIR}/src/sim/SimControlServer_test.cpp
)
target_link_libraries(wp_core_tests PRIVATE wp_core_lib GTest::gtest GTest::gtest_main)
target_compile_definitions(wp_core_tests PRIVATE
    WP_PLATFORM_DIR="${WP_PROTO_ROOT}/platform")
gtest_discover_tests(wp_core_tests)
