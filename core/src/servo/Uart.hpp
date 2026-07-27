#pragma once

#include <cstddef>
#include <cstdint>
#include <string>

#include <sys/types.h>  // ssize_t

namespace wp::servo {

class Uart {
public:
    virtual ~Uart() = default;
    virtual int open(const std::string& path, int baud) = 0;
    virtual ssize_t write(const std::uint8_t* data, std::size_t n) = 0;
    virtual ssize_t read(std::uint8_t* buf, std::size_t n, int timeoutMs) = 0;
    virtual void close() = 0;
    virtual void setHalfDuplexDirection(bool /*tx*/) {}
};

}  // namespace wp::servo
