#pragma once

#include <string>

#include "nats/Transport.hpp"

namespace wp::nats {

class UnixTransport : public Transport {
public:
    explicit UnixTransport(std::string path) : path_(std::move(path)) {}
    ~UnixTransport() override { close(); }

    int connect() override;
    ssize_t write(const char* data, std::size_t n) override;
    ssize_t read(char* buf, std::size_t n) override;
    void close() override;

private:
    std::string path_;
    int fd_ = -1;
};

}  // namespace wp::nats
