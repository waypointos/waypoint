#pragma once

#include <string>

namespace wp::config {

struct Args {
    std::string socketPath;
    std::string roverID = "sim-01";
    std::string servoPort = "/dev/ttyAMA0";
    std::string descriptorPath;
    bool servoMock = false;
    std::string simTime;
    std::string logLevel = "info";

    static Args parse(int argc, char** argv);
};

}  // namespace wp::config
