#include "util/Stamp.hpp"

#include <ctime>
#include <fstream>
#include <string>

namespace wp::util {

namespace {
const std::string& bootId() {
    static const std::string id = [] {
        std::ifstream f("/proc/sys/kernel/random/boot_id");
        std::string s;
        if (f.good()) std::getline(f, s);
        return s;
    }();
    return id;
}

std::function<std::uint64_t()> g_monoSrc;
}  // namespace

void setMonoNsSource(std::function<std::uint64_t()> src) {
    g_monoSrc = std::move(src);
}

void fillStamp(waypoint::v1::Stamp* s) {
    timespec wall{};
    clock_gettime(CLOCK_REALTIME, &wall);
    s->mutable_t()->set_seconds(wall.tv_sec);
    s->mutable_t()->set_nanos(static_cast<std::int32_t>(wall.tv_nsec));

    if (g_monoSrc) {
        s->set_mono_ns(g_monoSrc());
    } else {
        timespec mono{};
        clock_gettime(CLOCK_MONOTONIC, &mono);
        s->set_mono_ns(static_cast<std::uint64_t>(mono.tv_sec) * 1000000000ull +
                       static_cast<std::uint64_t>(mono.tv_nsec));
    }

    s->set_boot_id(bootId());
}

}  // namespace wp::util
