#pragma once

#include "servo/Uart.hpp"

namespace wp::servo {

class LinuxUart : public Uart {
public:
    LinuxUart() = default;
    ~LinuxUart() override { close(); }

    int open(const std::string& path, int baud) override;
    void close() override;
    ssize_t write(const std::uint8_t* data, std::size_t n) override;
    ssize_t read(std::uint8_t* buf, std::size_t n, int timeoutMs) override;

private:
    int fd_ = -1;
};

}  // namespace wp::servo
