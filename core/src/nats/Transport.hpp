#pragma once

#include <cstddef>
#include <string>

namespace wp::nats {

// Transport abstracts the byte-level link between the C++ core and the agent's
// NATS server. Exactly one implementation in v1: UnixTransport.
class Transport {
public:
    virtual ~Transport() = default;

    // Returns 0 or error code (errno). Sets up the connection synchronously.
    virtual int connect() = 0;

    // Returns bytes written, or -1 on error (errno set).
    virtual ssize_t write(const char* data, std::size_t n) = 0;

    // Blocking read into buf. Returns bytes read, 0 on EOF, -1 on error.
    virtual ssize_t read(char* buf, std::size_t n) = 0;

    virtual void close() = 0;
};

}  // namespace wp::nats
