#pragma once

// STS3215 Communication Protocol.
//
// Frame format:
//   0xFF 0xFF <id> <length> <instruction> <param1..paramN> <checksum>
//   length      = N + 2 (params + instruction + checksum)
//   checksum    = ~(id + length + instruction + sum(params)) & 0xFF
//
// Source: Waveshare STS3215 datasheet, "Servo Communication Protocol" section.

#include <cstdint>
#include <optional>
#include <utility>
#include <vector>

namespace wp::servo {

constexpr std::uint8_t kHeader[2] = {0xFF, 0xFF};

enum class Instruction : std::uint8_t {
    Ping        = 0x01,
    ReadData    = 0x02,
    WriteData   = 0x03,
    RegWrite    = 0x04,
    Action      = 0x05,
    Reset       = 0x06,
    SyncWrite   = 0x83,
};

// Register addresses we care about for the four drive wheels.
// Numbers match the datasheet; "L" = low byte, "H" = high byte.
namespace reg {
constexpr std::uint8_t RETURN_DELAY  = 0x07;  // 7  (EEPROM) bus response delay; 0 = min
constexpr std::uint8_t MIN_ANGLE_LIMIT = 0x09;  // 9  (L) / 0x0A (H)  EEPROM
constexpr std::uint8_t MAX_ANGLE_LIMIT = 0x0B;  // 11 (L) / 0x0C (H)  EEPROM
constexpr std::uint8_t COMP_P        = 0x15;  // 21 (EEPROM) position-loop P coefficient
constexpr std::uint8_t COMP_D        = 0x16;  // 22 (EEPROM) position-loop D coefficient
constexpr std::uint8_t COMP_I        = 0x17;  // 23 (EEPROM) position-loop I coefficient
constexpr std::uint8_t MODE          = 0x21;  // 33 (EEPROM) 0=position, 1=wheel, 2=PWM, 3=step
constexpr std::uint8_t TORQUE_ENABLE = 0x28;  // 40
constexpr std::uint8_t TORQUE_LIMIT  = 0x30;  // 48 (L) / 0x31 (H)  running cap (SRAM)
constexpr std::uint8_t GOAL_POSITION = 0x2A;  // 42 (L) / 43 (H)
constexpr std::uint8_t GOAL_TIME     = 0x2C;  // 44 (L) / 45 (H)
constexpr std::uint8_t GOAL_SPEED    = 0x2E;  // 46 (L) / 47 (H)
constexpr std::uint8_t ACCELERATION  = 0x29;  // 41 (SRAM, 1 byte) goal accel/decel ramp
constexpr std::uint8_t MAX_ACCELERATION = 0x55;  // 85 (L) / 86 (H)  SRAM (2 bytes) accel ceiling
constexpr std::uint8_t LOCK          = 0x37;  // 55: 0 = unlocked (EEPROM writes pass), 1 = locked
constexpr std::uint8_t PRESENT_POSITION = 0x38;  // 56 (L) / 57 (H)
constexpr std::uint8_t PRESENT_SPEED    = 0x3A;  // 58
constexpr std::uint8_t PRESENT_LOAD     = 0x3C;  // 60
constexpr std::uint8_t PRESENT_VOLTAGE  = 0x3E;  // 62
constexpr std::uint8_t PRESENT_TEMPERATURE = 0x3F;  // 63
constexpr std::uint8_t PRESENT_CURRENT  = 0x45;  // 69
}  // namespace reg

// Speed and load registers are sign-magnitude, not two's complement: a
// direction flag (bit 15 for speed, bit 10 for load) over an unsigned
// magnitude. Matches Feetech's SCServo reference (WriteSpe/ReadSpeed).
constexpr std::uint16_t encodeSignMagnitude(std::int16_t v, unsigned signBit) {
    return v < 0 ? static_cast<std::uint16_t>(-static_cast<std::int32_t>(v) | (1 << signBit))
                 : static_cast<std::uint16_t>(v);
}
constexpr std::int16_t decodeSignMagnitude(std::uint16_t raw, unsigned signBit) {
    const std::int16_t mag = static_cast<std::int16_t>(raw & ((1u << signBit) - 1));
    return (raw & (1u << signBit)) ? static_cast<std::int16_t>(-mag) : mag;
}

struct Frame {
    std::uint8_t id;
    Instruction instruction;
    std::vector<std::uint8_t> params;
};

std::uint8_t checksum(std::uint8_t id, std::uint8_t length, std::uint8_t instr,
                      const std::uint8_t* params, std::size_t paramLen);

// Serialize a frame to raw bytes.
std::vector<std::uint8_t> encode(const Frame& f);

// Try to decode the next frame from `buf`. Returns:
//   - nullopt if `buf` does not yet contain a complete frame
//   - the decoded Frame on success; consumed = bytes consumed (incl. header).
struct DecodeResult {
    Frame frame;
    std::size_t consumed = 0;
};
std::optional<DecodeResult> decode(const std::uint8_t* buf, std::size_t n);

// High-level builders (return ready-to-send byte buffers).
std::vector<std::uint8_t> buildPing(std::uint8_t id);
std::vector<std::uint8_t> buildReadPosition(std::uint8_t id);
std::vector<std::uint8_t> buildReadStateBlock(std::uint8_t id);   // pos+speed+load+volt+temp+current
std::vector<std::uint8_t> buildReadMode(std::uint8_t id);
std::vector<std::uint8_t> buildSetGoalSpeed(std::uint8_t id, std::int16_t signedSpeed);
std::vector<std::uint8_t> buildSetTorqueEnable(std::uint8_t id, bool on);
std::vector<std::uint8_t> buildSetMode(std::uint8_t id, std::uint8_t mode);
std::vector<std::uint8_t> buildSetGoalPosition(std::uint8_t id, std::uint16_t raw);
std::vector<std::uint8_t> buildSetTorqueLimit(std::uint8_t id, std::uint16_t raw);
std::vector<std::uint8_t> buildSetAngleLimits(std::uint8_t id, std::uint16_t minRaw, std::uint16_t maxRaw);
std::vector<std::uint8_t> buildSyncWriteGoalPositions(
    const std::vector<std::pair<std::uint8_t, std::uint16_t>>& goals);
std::vector<std::uint8_t> buildSetEEPROMLock(std::uint8_t id, bool locked);

// Generic single-register writers used for the tuning registers (PID, accel,
// return-delay). buildWriteWord encodes value little-endian to match the
// GOAL_POSITION encoding (low byte first).
std::vector<std::uint8_t> buildWriteByte(std::uint8_t id, std::uint8_t reg, std::uint8_t value);
std::vector<std::uint8_t> buildWriteWord(std::uint8_t id, std::uint8_t reg, std::uint16_t value);

}  // namespace wp::servo
