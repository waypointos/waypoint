#pragma once

#include <cstddef>
#include <cstdint>
#include <optional>
#include <string>
#include <vector>

namespace wp::nats {

enum class FrameKind { Info, Msg, Ping, Pong, Ok, Err };

struct Frame {
    FrameKind kind;
    std::string subject;   // MSG only
    std::string sid;       // MSG only
    std::string reply;     // MSG only (5-part form)
    std::string payload;   // MSG: body. INFO/-ERR: text. Others: empty.
};

// Parser is a single-threaded text-line accumulator. Feed it bytes; pull
// completed frames via next(). Partial frames are buffered internally.
class Parser {
public:
    void feed(const char* data, std::size_t n);
    void feed(const std::string& s) { feed(s.data(), s.size()); }
    std::optional<Frame> next();

private:
    std::string buf_;
    std::size_t pendingPayloadLen_ = 0;
    Frame pendingMsg_;
    bool waitingForPayload_ = false;
};

// Free-function serializers.
std::string serializeConnect(bool verbose, bool pedantic);
std::string serializePub(const std::string& subject, const std::string& reply,
                         const char* data, std::size_t n);
std::string serializeSub(const std::string& subject, const std::string& sid);
std::string serializeUnsub(const std::string& sid, std::optional<int> maxMsgs = std::nullopt);
std::string serializePong();

}  // namespace wp::nats
