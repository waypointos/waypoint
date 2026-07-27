#include "config/Args.hpp"

#include <cstring>
#include <iostream>

namespace wp::config {

namespace {
const char* defaultSocketPath() {
#ifdef __APPLE__
    return "/tmp/waypoint-nats.sock";
#else
    return "/run/waypoint/nats.sock";
#endif
}

bool takesValue(const char* opt) {
    return std::strcmp(opt, "--socket") == 0
        || std::strcmp(opt, "--rover-id") == 0
        || std::strcmp(opt, "--servo-port") == 0
        || std::strcmp(opt, "--descriptor") == 0
        || std::strcmp(opt, "--sim-time") == 0
        || std::strcmp(opt, "--log-level") == 0;
}
}  // namespace

Args Args::parse(int argc, char** argv) {
    Args a;
    a.socketPath = defaultSocketPath();

    for (int i = 1; i < argc; ++i) {
        const char* opt = argv[i];
        if (takesValue(opt)) {
            if (i + 1 >= argc) {
                std::cerr << "waypoint-core: " << opt << " requires a value\n";
                std::exit(2);
            }
            const char* val = argv[++i];
            if (std::strcmp(opt, "--socket") == 0)      a.socketPath = val;
            else if (std::strcmp(opt, "--rover-id") == 0)   a.roverID = val;
            else if (std::strcmp(opt, "--servo-port") == 0) a.servoPort = val;
            else if (std::strcmp(opt, "--descriptor") == 0) a.descriptorPath = val;
            else if (std::strcmp(opt, "--sim-time") == 0)   a.simTime = val;
            else if (std::strcmp(opt, "--log-level") == 0)  a.logLevel = val;
        } else if (std::strcmp(opt, "--servo-mock") == 0) {
            a.servoMock = true;
            a.servoPort = "mock";
        } else {
            std::cerr << "waypoint-core: unknown argument '" << opt << "'\n";
            std::exit(2);
        }
    }
    if (!a.simTime.empty() && a.simTime != "controlled") {
        std::cerr << "waypoint-core: --sim-time accepts only \"controlled\"\n";
        std::exit(2);
    }
    return a;
}

}  // namespace wp::config
