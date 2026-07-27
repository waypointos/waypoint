#include "servo/LinuxUart.hpp"

#include <fcntl.h>
#include <poll.h>
#include <termios.h>
#include <unistd.h>

#include <cerrno>
#include <cstring>

namespace wp::servo {

namespace {
speed_t toSpeed(int baud) {
    switch (baud) {
    case 9600: return B9600;
    case 19200: return B19200;
    case 57600: return B57600;
    case 115200: return B115200;
    case 230400: return B230400;
#ifdef B460800
    case 460800: return B460800;
#endif
#ifdef B921600
    case 921600: return B921600;
#endif
#ifdef B1000000
    case 1000000: return B1000000;  // STS3215 default 1 Mbps
#endif
    default:
#ifdef B1000000
        return B1000000;
#else
        return B115200;
#endif
    }
}
}  // namespace

int LinuxUart::open(const std::string& path, int baud) {
    fd_ = ::open(path.c_str(), O_RDWR | O_NOCTTY | O_NDELAY);
    if (fd_ < 0) return errno;

    termios tty{};
    if (tcgetattr(fd_, &tty) != 0) {
        int e = errno;
        ::close(fd_);
        fd_ = -1;
        return e;
    }

    cfmakeraw(&tty);
    cfsetspeed(&tty, toSpeed(baud));
    tty.c_cflag |= (CLOCAL | CREAD);
    tty.c_cflag &= ~CSTOPB;
    tty.c_cflag &= ~CRTSCTS;
    tty.c_cflag &= ~PARENB;
    tty.c_cc[VMIN] = 0;
    tty.c_cc[VTIME] = 0;

    if (tcsetattr(fd_, TCSANOW, &tty) != 0) {
        int e = errno;
        ::close(fd_);
        fd_ = -1;
        return e;
    }
    return 0;
}

void LinuxUart::close() {
    if (fd_ >= 0) {
        ::close(fd_);
        fd_ = -1;
    }
}

ssize_t LinuxUart::write(const std::uint8_t* data, std::size_t n) {
    return ::write(fd_, data, n);
}

ssize_t LinuxUart::read(std::uint8_t* buf, std::size_t n, int timeoutMs) {
    pollfd p{fd_, POLLIN, 0};
    int r = ::poll(&p, 1, timeoutMs);
    if (r <= 0) return 0;
    return ::read(fd_, buf, n);
}

}  // namespace wp::servo
