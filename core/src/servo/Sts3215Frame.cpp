#include "servo/Sts3215Frame.hpp"

namespace wp::servo {

std::uint8_t checksum(std::uint8_t id, std::uint8_t length, std::uint8_t instr,
                      const std::uint8_t* params, std::size_t paramLen) {
    std::uint32_t sum = id + length + instr;
    for (std::size_t i = 0; i < paramLen; ++i) sum += params[i];
    return static_cast<std::uint8_t>(~(sum & 0xFF));
}

std::vector<std::uint8_t> encode(const Frame& f) {
    const std::uint8_t length = static_cast<std::uint8_t>(f.params.size() + 2);
    std::vector<std::uint8_t> out;
    out.reserve(6 + f.params.size());
    out.push_back(0xFF);
    out.push_back(0xFF);
    out.push_back(f.id);
    out.push_back(length);
    out.push_back(static_cast<std::uint8_t>(f.instruction));
    out.insert(out.end(), f.params.begin(), f.params.end());
    out.push_back(checksum(f.id, length, static_cast<std::uint8_t>(f.instruction),
                           f.params.data(), f.params.size()));
    return out;
}

std::optional<DecodeResult> decode(const std::uint8_t* buf, std::size_t n) {
    if (n < 6) return std::nullopt;
    if (buf[0] != 0xFF || buf[1] != 0xFF) return std::nullopt;
    std::uint8_t id = buf[2];
    std::uint8_t length = buf[3];
    if (length < 2) return std::nullopt;
    std::size_t total = 4 + length;
    if (n < total) return std::nullopt;

    std::uint8_t err = buf[4];
    std::size_t paramLen = length - 2;
    const std::uint8_t* params = buf + 5;
    std::uint8_t cs = checksum(id, length, err, params, paramLen);
    if (cs != buf[4 + length - 1]) return std::nullopt;

    DecodeResult r;
    r.frame.id = id;
    r.frame.instruction = Instruction::Ping;  // response frames don't carry instr; we set Ping arbitrarily
    r.frame.params.assign(buf + 4, buf + 4 + length - 1);
    r.consumed = total;
    return r;
}

std::vector<std::uint8_t> buildPing(std::uint8_t id) {
    return encode({id, Instruction::Ping, {}});
}

std::vector<std::uint8_t> buildReadPosition(std::uint8_t id) {
    return encode({id, Instruction::ReadData, {reg::PRESENT_POSITION, 0x02}});
}

std::vector<std::uint8_t> buildReadStateBlock(std::uint8_t id) {
    // Read 15 bytes 0x38..0x46: pos, speed, load, voltage, temperature, (gap), present-current LH.
    // PRESENT_CURRENT_H lives at 0x46 so the block must extend one byte past 0x45.
    return encode({id, Instruction::ReadData, {reg::PRESENT_POSITION, 0x0F}});
}

std::vector<std::uint8_t> buildSetGoalSpeed(std::uint8_t id, std::int16_t signedSpeed) {
    std::uint16_t v = encodeSignMagnitude(signedSpeed, 15);
    return encode({id, Instruction::WriteData,
                   {reg::GOAL_SPEED,
                    static_cast<std::uint8_t>(v & 0xFF),
                    static_cast<std::uint8_t>((v >> 8) & 0xFF)}});
}

std::vector<std::uint8_t> buildSetTorqueEnable(std::uint8_t id, bool on) {
    return encode({id, Instruction::WriteData,
                   {reg::TORQUE_ENABLE, static_cast<std::uint8_t>(on ? 1 : 0)}});
}

std::vector<std::uint8_t> buildReadMode(std::uint8_t id) {
    return encode({id, Instruction::ReadData, {reg::MODE, 0x01}});
}

std::vector<std::uint8_t> buildSetMode(std::uint8_t id, std::uint8_t mode) {
    return encode({id, Instruction::WriteData, {reg::MODE, mode}});
}

std::vector<std::uint8_t> buildSetGoalPosition(std::uint8_t id, std::uint16_t raw) {
    return encode({id, Instruction::WriteData,
                   {reg::GOAL_POSITION,
                    static_cast<std::uint8_t>(raw & 0xFF),
                    static_cast<std::uint8_t>((raw >> 8) & 0xFF)}});
}

std::vector<std::uint8_t> buildSetTorqueLimit(std::uint8_t id, std::uint16_t raw) {
    return encode({id, Instruction::WriteData,
                   {reg::TORQUE_LIMIT,
                    static_cast<std::uint8_t>(raw & 0xFF),
                    static_cast<std::uint8_t>((raw >> 8) & 0xFF)}});
}

std::vector<std::uint8_t> buildSetAngleLimits(std::uint8_t id, std::uint16_t minRaw, std::uint16_t maxRaw) {
    return encode({id, Instruction::WriteData,
                   {reg::MIN_ANGLE_LIMIT,
                    static_cast<std::uint8_t>(minRaw & 0xFF),
                    static_cast<std::uint8_t>((minRaw >> 8) & 0xFF),
                    static_cast<std::uint8_t>(maxRaw & 0xFF),
                    static_cast<std::uint8_t>((maxRaw >> 8) & 0xFF)}});
}

std::vector<std::uint8_t> buildSyncWriteGoalPositions(
    const std::vector<std::pair<std::uint8_t, std::uint16_t>>& goals) {
    constexpr std::uint8_t kBroadcast = 0xFE;
    constexpr std::uint8_t kDataLen = 2;  // GOAL_POSITION is 2 bytes
    const std::uint8_t len = static_cast<std::uint8_t>((kDataLen + 1) * goals.size() + 4);

    std::vector<std::uint8_t> params;
    params.push_back(reg::GOAL_POSITION);
    params.push_back(kDataLen);
    for (const auto& [id, raw] : goals) {
        params.push_back(id);
        params.push_back(static_cast<std::uint8_t>(raw & 0xFF));
        params.push_back(static_cast<std::uint8_t>((raw >> 8) & 0xFF));
    }

    std::vector<std::uint8_t> out{0xFF, 0xFF, kBroadcast, len,
                                  static_cast<std::uint8_t>(Instruction::SyncWrite)};
    out.insert(out.end(), params.begin(), params.end());
    out.push_back(checksum(kBroadcast, len,
                           static_cast<std::uint8_t>(Instruction::SyncWrite),
                           params.data(), params.size()));
    return out;
}

std::vector<std::uint8_t> buildSetEEPROMLock(std::uint8_t id, bool locked) {
    return encode({id, Instruction::WriteData,
                   {reg::LOCK, static_cast<std::uint8_t>(locked ? 1 : 0)}});
}

std::vector<std::uint8_t> buildWriteByte(std::uint8_t id, std::uint8_t reg, std::uint8_t value) {
    return encode({id, Instruction::WriteData, {reg, value}});
}

std::vector<std::uint8_t> buildWriteWord(std::uint8_t id, std::uint8_t reg, std::uint16_t value) {
    return encode({id, Instruction::WriteData,
                   {reg,
                    static_cast<std::uint8_t>(value & 0xFF),
                    static_cast<std::uint8_t>((value >> 8) & 0xFF)}});
}

}  // namespace wp::servo
