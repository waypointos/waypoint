# Generates C++ bindings from protocol/messages/*.proto into the build tree.
# Outputs are added to a static library wp_protocol_cpp.
#
# Proto layout: the protocol/ module roots imports at protocol/ (per buf.yaml),
# and proto files live under protocol/messages/. We mirror that import path
# under gen/cpp so includes look like "messages/drive.pb.h".

function(wp_generate_protobuf)
    # WP_PROTO_ROOT lets out-of-tree builders (Buildroot, CI) point at the
    # protocol/ tree directly. Local dev builds default to the sibling
    # directory ../protocol, which is where it lives in the repo layout.
    if(NOT DEFINED WP_PROTO_ROOT OR WP_PROTO_ROOT STREQUAL "")
        set(WP_PROTO_ROOT ${CMAKE_SOURCE_DIR}/../protocol)
    endif()
    set(PROTO_IMPORT_ROOT ${WP_PROTO_ROOT})
    set(PROTO_SRC_DIR ${PROTO_IMPORT_ROOT}/messages)
    file(GLOB PROTO_FILES ${PROTO_SRC_DIR}/*.proto)
    set(GEN_DIR ${CMAKE_BINARY_DIR}/gen/cpp)
    file(MAKE_DIRECTORY ${GEN_DIR})

    set(GEN_SOURCES "")
    foreach(PROTO ${PROTO_FILES})
        get_filename_component(NAME ${PROTO} NAME_WE)
        set(OUT_CC ${GEN_DIR}/messages/${NAME}.pb.cc)
        set(OUT_H  ${GEN_DIR}/messages/${NAME}.pb.h)
        add_custom_command(
            OUTPUT ${OUT_CC} ${OUT_H}
            COMMAND ${Protobuf_PROTOC_EXECUTABLE}
                    --cpp_out=${GEN_DIR}
                    -I ${PROTO_IMPORT_ROOT}
                    messages/${NAME}.proto
            DEPENDS ${PROTO}
            WORKING_DIRECTORY ${PROTO_IMPORT_ROOT}
        )
        list(APPEND GEN_SOURCES ${OUT_CC})
    endforeach()

    add_library(wp_protocol_cpp STATIC ${GEN_SOURCES})
    target_include_directories(wp_protocol_cpp PUBLIC ${GEN_DIR})
    target_link_libraries(wp_protocol_cpp PUBLIC protobuf::libprotobuf)
    # Proto messages with map<> fields (e.g. AlertRaised.metadata) pull in
    # absl symbols from the generated TUs; link absl when available so the
    # link doesn't fail with undefined absl::log_internal / hash_internal refs.
    find_package(absl QUIET)
    if(absl_FOUND)
        target_link_libraries(wp_protocol_cpp PUBLIC
            absl::log_internal_message
            absl::log_internal_check_op
            absl::hash
            absl::raw_hash_set
            absl::strings)
    endif()
    # Generated protobuf code emits warnings under strict flags; turn them off
    # for the generated translation units only.
    target_compile_options(wp_protocol_cpp PRIVATE -w)
endfunction()
