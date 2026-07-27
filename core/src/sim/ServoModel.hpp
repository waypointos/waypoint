#pragma once

#include <array>
#include <cstdint>
#include <map>
#include <mutex>
#include <optional>
#include <random>
#include <utility>
#include <vector>

namespace wp::sim {

struct ServoConfig {
    std::uint8_t id = 0;
    bool wheelMode = false;  // initial operating mode (descriptor joint type)
    bool present = true;
};

// Deterministic STS3215 behavior model. Frame-agnostic: SimUart translates
// protocol frames into the register read/write calls below. Physics advances
// only in step(), driven by the loop on the virtual clock. Fidelity contract:
// every behavior here encodes a recorded hardware fact;
// tests are named for the fact they pin.
// Thread-safe: scenario setters run on the NATS reader thread while the loop
// thread steps and the bus reads; one internal mutex serializes them.
class ServoModel {
public:
    ServoModel(std::vector<ServoConfig> servos, std::uint64_t seed);

    void step(double dtSeconds);

    bool present(std::uint8_t id) const;
    bool declared(std::uint8_t id) const;
    // Returns false for absent/unknown servos. EEPROM registers respect LOCK.
    bool writeReg(std::uint8_t id, std::uint8_t addr, const std::uint8_t* data, std::size_t n);
    // nullopt for absent/unknown servos or out-of-range reads.
    std::optional<std::vector<std::uint8_t>> readRegs(std::uint8_t id, std::uint8_t addr,
                                                      std::uint8_t len);

    // Scenario surface (cmd.sim_scenario maps onto these).
    void setAbsent(std::uint8_t id, bool absent);
    void setHardStops(std::uint8_t id, std::uint16_t minTicks, std::uint16_t maxTicks);
    void setStallCurrent(std::uint8_t id, double amps);
    void setTemperatureOffset(std::uint8_t id, double c);
    void setVoltageSagFactor(double f);
    void resetSeed(std::uint64_t seed);
    void clearOverrides();

    std::uint64_t seed() const;
    std::uint32_t overrideCount() const;

private:
    struct Servo {
        ServoConfig cfg;
        std::array<std::uint8_t, 0x50> regs{};
        double positionTicks = 2048.0;
        double velocityRadps = 0.0;
        double temperatureC = 25.0;
        double currentA = 0.0;
        std::optional<std::pair<std::uint16_t, std::uint16_t>> hardStops;
        double stallCurrentA = 2.0;
        double temperatureOffsetC = 0.0;
        bool overridden = false;
    };
    Servo* find(std::uint8_t id);
    const Servo* find(std::uint8_t id) const;
    void mirrorPresentRegs(Servo& s);

    mutable std::mutex mu_;
    std::map<std::uint8_t, Servo> servos_;
    double voltageSagFactor_ = 1.0;
    double busVoltageV_ = 12.4;
    std::uint64_t seed_;
    std::mt19937_64 rng_;
};

}  // namespace wp::sim
