add_library(wp_core_lib STATIC
    nats/Frame.cpp
    nats/UnixTransport.cpp
    nats/Connection.cpp
    servo/Sts3215Frame.cpp
    servo/MockUart.cpp
    servo/LinuxUart.cpp
    servo/Sts3215Bus.cpp
    servo/AuxRotation.cpp
    servo/Discovery.cpp
    servo/ServoControlServer.cpp
    control/DiffDriveKinematics.cpp
    control/DriveController.cpp
    mode/ModeMachine.cpp
    watchdog/Watchdog.cpp
    telemetry/DrivePublisher.cpp
    telemetry/MotorPublisher.cpp
    telemetry/PlatformPublisher.cpp
    telemetry/PowerPublisher.cpp
    telemetry/ServoStateCache.cpp
    rpc/SetModeHandler.cpp
    rpc/EstopHandler.cpp
    rpc/RecoverHandler.cpp
    rpc/GetInfoHandler.cpp
    config/Args.cpp
    platform/Descriptor.cpp
    joint/Sts3215Driver.cpp
    util/Stamp.cpp
    util/Clock.cpp
    sim/ServoModel.cpp
    sim/SimUart.cpp
    sim/SimControlServer.cpp
)
target_include_directories(wp_core_lib PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
target_include_directories(wp_core_lib SYSTEM PUBLIC ${CMAKE_SOURCE_DIR}/third_party)
target_link_libraries(wp_core_lib PUBLIC wp_protocol_cpp)

add_executable(waypoint-core main.cpp)
target_link_libraries(waypoint-core PRIVATE wp_core_lib)
