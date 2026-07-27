#include "nats/UnixTransport.hpp"

#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

#include <cerrno>
#include <cstring>

namespace wp::nats {

int UnixTransport::connect() {
    fd_ = ::socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd_ < 0) return errno;

    sockaddr_un addr{};
    addr.sun_family = AF_UNIX;
    std::strncpy(addr.sun_path, path_.c_str(), sizeof(addr.sun_path) - 1);

    if (::connect(fd_, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
        int e = errno;
        ::close(fd_);
        fd_ = -1;
        return e;
    }
    return 0;
}

ssize_t UnixTransport::write(const char* data, std::size_t n) {
    if (fd_ < 0) return -1;
    return ::write(fd_, data, n);
}

ssize_t UnixTransport::read(char* buf, std::size_t n) {
    if (fd_ < 0) return -1;
    return ::read(fd_, buf, n);
}

void UnixTransport::close() {
    if (fd_ >= 0) {
        ::close(fd_);
        fd_ = -1;
    }
}

}  // namespace wp::nats
