#include "nats/Frame.hpp"

#include <algorithm>
#include <cstring>
#include <sstream>

namespace wp::nats {

namespace {

bool startsWith(const std::string& s, const char* prefix) {
    auto n = std::strlen(prefix);
    return s.size() >= n && std::memcmp(s.data(), prefix, n) == 0;
}

std::size_t findCRLF(const std::string& s, std::size_t start) {
    for (std::size_t i = start; i + 1 < s.size(); ++i) {
        if (s[i] == '\r' && s[i + 1] == '\n') return i;
    }
    return std::string::npos;
}

std::vector<std::string> splitSpace(const std::string& s) {
    std::vector<std::string> out;
    std::istringstream iss(s);
    std::string tok;
    while (iss >> tok) out.push_back(tok);
    return out;
}

}  // namespace

void Parser::feed(const char* data, std::size_t n) {
    buf_.append(data, n);
}

std::optional<Frame> Parser::next() {
    if (waitingForPayload_) {
        if (buf_.size() < pendingPayloadLen_ + 2) return std::nullopt;
        pendingMsg_.payload.assign(buf_.data(), pendingPayloadLen_);
        // Consume payload + trailing \r\n
        buf_.erase(0, pendingPayloadLen_ + 2);
        waitingForPayload_ = false;
        Frame out = std::move(pendingMsg_);
        pendingMsg_ = Frame{};
        return out;
    }

    auto crlf = findCRLF(buf_, 0);
    if (crlf == std::string::npos) return std::nullopt;

    std::string line = buf_.substr(0, crlf);
    buf_.erase(0, crlf + 2);

    if (line.empty()) return std::nullopt;

    if (startsWith(line, "INFO ")) {
        Frame f;
        f.kind = FrameKind::Info;
        f.payload = line.substr(5);
        return f;
    }
    if (line == "PING") {
        Frame f;
        f.kind = FrameKind::Ping;
        return f;
    }
    if (line == "PONG") {
        Frame f;
        f.kind = FrameKind::Pong;
        return f;
    }
    if (line == "+OK") {
        Frame f;
        f.kind = FrameKind::Ok;
        return f;
    }
    if (startsWith(line, "-ERR ")) {
        Frame f;
        f.kind = FrameKind::Err;
        f.payload = line.substr(5);
        return f;
    }
    if (startsWith(line, "MSG ")) {
        auto tokens = splitSpace(line.substr(4));
        // MSG <subject> <sid> [reply] <size>
        if (tokens.size() < 3 || tokens.size() > 4) return std::nullopt;
        pendingMsg_ = Frame{};
        pendingMsg_.kind = FrameKind::Msg;
        pendingMsg_.subject = tokens[0];
        pendingMsg_.sid = tokens[1];
        if (tokens.size() == 4) {
            pendingMsg_.reply = tokens[2];
            pendingPayloadLen_ = static_cast<std::size_t>(std::stoul(tokens[3]));
        } else {
            pendingPayloadLen_ = static_cast<std::size_t>(std::stoul(tokens[2]));
        }
        waitingForPayload_ = true;
        return next();  // tail-recurse if payload already buffered
    }

    // Unknown line — drop, return next.
    return next();
}

std::string serializeConnect(bool verbose, bool pedantic) {
    std::ostringstream o;
    o << "CONNECT {\"verbose\":" << (verbose ? "true" : "false")
      << ",\"pedantic\":" << (pedantic ? "true" : "false")
      << ",\"name\":\"waypoint-core\",\"lang\":\"cpp\",\"version\":\"0.4.0\"}\r\n";
    return o.str();
}

std::string serializePub(const std::string& subject, const std::string& reply,
                         const char* data, std::size_t n) {
    std::ostringstream o;
    o << "PUB " << subject;
    if (!reply.empty()) o << " " << reply;
    o << " " << n << "\r\n";
    std::string head = o.str();
    head.append(data, n);
    head.append("\r\n");
    return head;
}

std::string serializeSub(const std::string& subject, const std::string& sid) {
    return "SUB " + subject + " " + sid + "\r\n";
}

std::string serializeUnsub(const std::string& sid, std::optional<int> maxMsgs) {
    std::string s = "UNSUB " + sid;
    if (maxMsgs) s += " " + std::to_string(*maxMsgs);
    s += "\r\n";
    return s;
}

std::string serializePong() { return "PONG\r\n"; }

}  // namespace wp::nats
