#include "sim/SimUart.hpp"

#include <optional>

#include "servo/Sts3215Frame.hpp"

namespace wp::sim {

namespace {

// servo::decode parses STATUS frames (byte[4] = err, instruction discarded),
// so it cannot recover the instruction of a command frame. Parse command
// frames here: FF FF id len instr params... cs, checksummed over the instr.
struct Parsed {
    servo::Frame frame;
    std::size_t consumed = 0;
};

std::optional<Parsed> tryParseInstruction(const std::uint8_t* buf, std::size_t n) {
    if (n < 6) return std::nullopt;
    if (buf[0] != 0xFF || buf[1] != 0xFF) return std::nullopt;
    std::uint8_t id = buf[2];
    std::uint8_t length = buf[3];
    if (length < 2) return std::nullopt;
    std::size_t total = 4 + length;
    if (n < total) return std::nullopt;

    std::uint8_t instr = buf[4];
    std::size_t paramLen = length - 2;
    const std::uint8_t* params = buf + 5;
    std::uint8_t cs = servo::checksum(id, length, instr, params, paramLen);
    if (cs != buf[total - 1]) return std::nullopt;

    Parsed p;
    p.frame.id = id;
    p.frame.instruction = static_cast<servo::Instruction>(instr);
    p.frame.params.assign(params, params + paramLen);
    p.consumed = total;
    return p;
}

}  // namespace

SimUart::SimUart(std::shared_ptr<ServoModel> model) : model_(std::move(model)) {}

ssize_t SimUart::write(const std::uint8_t* data, std::size_t n) {
    // Half-duplex echo: TX bytes return on RX before any response, but only
    // when the line is in RX. The bus drives TX during write, so its reply read
    // sees only the response (matching the real transceiver gating the echo).
    if (!txMode_) {
        for (std::size_t i = 0; i < n; ++i) rx_.push_back(data[i]);
    }
    txAccum_.insert(txAccum_.end(), data, data + n);

    while (!txAccum_.empty()) {
        auto p = tryParseInstruction(txAccum_.data(), txAccum_.size());
        if (!p) {
            // No complete frame. If the buffer cannot start a frame, resync by
            // dropping to the next 0xFF 0xFF candidate and count the bad byte.
            if (txAccum_.size() >= 2 &&
                !(txAccum_[0] == 0xFF && txAccum_[1] == 0xFF)) {
                ++malformed_;
                txAccum_.erase(txAccum_.begin());
                continue;
            }
            break;
        }
        dispatch(p->frame);
        txAccum_.erase(txAccum_.begin(),
                       txAccum_.begin() + static_cast<long>(p->consumed));
    }
    return static_cast<ssize_t>(n);
}

ssize_t SimUart::read(std::uint8_t* buf, std::size_t n, int /*timeoutMs*/) {
    std::size_t i = 0;
    while (i < n && !rx_.empty()) {
        buf[i++] = rx_.front();
        rx_.pop_front();
    }
    return static_cast<ssize_t>(i);
}

void SimUart::respond(std::uint8_t id, const std::vector<std::uint8_t>& params) {
    // Status frame: FF FF id len err params cs (err always 0 in sim).
    std::uint8_t len = static_cast<std::uint8_t>(params.size() + 2);
    std::uint8_t cs = servo::checksum(id, len, 0x00, params.data(), params.size());
    rx_.push_back(0xFF); rx_.push_back(0xFF); rx_.push_back(id); rx_.push_back(len);
    rx_.push_back(0x00);
    for (auto b : params) rx_.push_back(b);
    rx_.push_back(cs);
}

void SimUart::dispatch(const servo::Frame& f) {
    const bool broadcast = f.id == 0xFE;
    switch (f.instruction) {
    case servo::Instruction::Ping:
        if (!broadcast && model_->present(f.id)) respond(f.id, {});
        break;
    case servo::Instruction::ReadData: {
        if (broadcast || f.params.size() < 2) break;
        auto data = model_->readRegs(f.id, f.params[0], f.params[1]);
        if (data) respond(f.id, *data);
        break;
    }
    case servo::Instruction::WriteData: {
        if (f.params.size() < 1) break;
        if (broadcast) break;  // broadcast writes: applied nowhere in sim v1
        if (model_->writeReg(f.id, f.params[0], f.params.data() + 1,
                             f.params.size() - 1)) {
            respond(f.id, {});
        }
        break;
    }
    case servo::Instruction::SyncWrite: {
        // params: addr, perServoLen, then per servo: id + perServoLen bytes.
        if (f.params.size() < 2) { ++malformed_; break; }
        std::uint8_t addr = f.params[0];
        std::uint8_t per = f.params[1];
        std::size_t i = 2;
        while (i + 1 + per <= f.params.size()) {
            std::uint8_t sid = f.params[i];
            model_->writeReg(sid, addr, f.params.data() + i + 1, per);
            i += 1 + per;
        }
        break;  // broadcast: no response, matching hardware
    }
    default:
        break;  // unsupported instructions are silently accepted
    }
}

}  // namespace wp::sim
