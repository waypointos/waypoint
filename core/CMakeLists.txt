cmake_minimum_required(VERSION 3.22)
project(waypoint_core LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

if(NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release CACHE STRING "" FORCE)
endif()

add_compile_options(-Wall -Wextra -Wpedantic -Werror -Wno-error=unused-parameter)

option(WP_CORE_BUILD_TESTS "Build gtest test binaries" ON)

if(NOT DEFINED WP_PROTO_ROOT)
    set(WP_PROTO_ROOT ${CMAKE_SOURCE_DIR}/../protocol CACHE PATH "Path to the protocol/ directory")
endif()

# Find protobuf and (lazily) gtest
find_package(Protobuf REQUIRED)
if(WP_CORE_BUILD_TESTS)
    find_package(GTest REQUIRED)
    include(GoogleTest)
    enable_testing()
endif()

include(cmake/Protobuf.cmake)
wp_generate_protobuf()

add_subdirectory(src)
if(WP_CORE_BUILD_TESTS)
    add_subdirectory(tests)
endif()
