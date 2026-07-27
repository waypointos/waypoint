#include "nats/Connection.hpp"

#include <chrono>
#include <iostream>
#include <thread>
#include <utility>
#include <vector>

namespace wp::nats {

Connection::Connection(std::unique_ptr<Transport> t) : transport_(std::move(t)) {}

Connection::~Connection() { stop(); }

int Connection::start() {
    if (int e = transport_->connect()) return e;
    auto conn = serializeConnect(false, false);
    {
        std::lock_guard<std::mutex> lk(writeMu_);
        if (transport_->write(conn.data(), conn.size()) < 0) return -1;
    }
    running_.store(true);
    reader_ = std::thread([this] { readLoop(); });
    return 0;
}

void Connection::stop() {
    bool was = running_.exchange(false);
    if (!was) return;
    transport_->close();
    if (reader_.joinable()) reader_.join();
}

std::uint64_t Connection::subscribe(const std::string& subject, MsgHandler cb) {
    auto sid = nextSid_.fetch_add(1);
    {
        std::lock_guard<std::mutex> lk(subsMu_);
        subs_[sid] = {subject, std::move(cb)};
    }
    auto line = serializeSub(subject, std::to_string(sid));
    {
        std::lock_guard<std::mutex> lk(writeMu_);
        transport_->write(line.data(), line.size());
    }
    return sid;
}

int Connection::publish(const std::string& subject, const char* data, std::size_t n) {
    return publishRequest(subject, "", data, n);
}

int Connection::publishRequest(const std::string& subject, const std::string& reply,
                               const char* data, std::size_t n) {
    auto frame = serializePub(subject, reply, data, n);
    std::lock_guard<std::mutex> lk(writeMu_);
    if (transport_->write(frame.data(), frame.size()) < 0) return -1;
    return 0;
}

void Connection::reconnectLoop() {
    using namespace std::chrono_literals;
    while (running_.load()) {
        std::this_thread::sleep_for(1s);
        if (!running_.load()) return;

        // Close and reconnect the transport under writeMu_ so that any
        // concurrent publish sees a consistent fd state.
        {
            std::lock_guard<std::mutex> lk(writeMu_);
            transport_->close();
        }
        if (transport_->connect() != 0) {
            std::cerr << "[nats] reconnect failed, retrying in 1s\n";
            continue;
        }

        // Snapshot the subscriptions first (no write lock held), then send
        // CONNECT + SUBs under writeMu_. This avoids holding both mutexes
        // simultaneously which would invert the lock order used in subscribe().
        std::vector<std::pair<std::uint64_t, std::string>> subSnapshot;
        {
            std::lock_guard<std::mutex> slk(subsMu_);
            for (auto& [sid, entry] : subs_) {
                subSnapshot.emplace_back(sid, entry.first);
            }
        }
        {
            std::lock_guard<std::mutex> wlk(writeMu_);
            auto conn = serializeConnect(false, false);
            if (transport_->write(conn.data(), conn.size()) < 0) {
                std::cerr << "[nats] reconnect write CONNECT failed, retrying\n";
                continue;
            }
            for (auto& [sid, subject] : subSnapshot) {
                auto line = serializeSub(subject, std::to_string(sid));
                transport_->write(line.data(), line.size());
            }
        }

        std::cerr << "[nats] reconnected\n";
        // Reset the parser so stale partial frames from the old connection
        // don't corrupt the new session.
        parser_ = Parser{};
        return;
    }
}

void Connection::readLoop() {
    char buf[8192];
    while (running_.load()) {
        ssize_t n = transport_->read(buf, sizeof(buf));
        if (n <= 0) {
            if (!running_.load()) return;
            std::cerr << "[nats] transport closed, attempting reconnect\n";
            reconnectLoop();
            // If reconnectLoop returned because running_ went false, the outer
            // while condition catches that on the next iteration.
            continue;
        }
        parser_.feed(buf, static_cast<std::size_t>(n));
        while (auto fr = parser_.next()) {
            onFrame(*fr);
        }
    }
}

void Connection::onFrame(const Frame& f) {
    switch (f.kind) {
    case FrameKind::Ping: {
        auto pong = serializePong();
        std::lock_guard<std::mutex> lk(writeMu_);
        transport_->write(pong.data(), pong.size());
        return;
    }
    case FrameKind::Msg: {
        std::uint64_t sid = 0;
        try {
            sid = static_cast<std::uint64_t>(std::stoull(f.sid));
        } catch (...) {
            return;
        }
        MsgHandler cb;
        {
            std::lock_guard<std::mutex> lk(subsMu_);
            auto it = subs_.find(sid);
            if (it == subs_.end()) return;
            cb = it->second.second;
        }
        if (cb) {
            cb(Message{f.subject, f.reply, f.payload});
        }
        return;
    }
    case FrameKind::Err:
        std::cerr << "[nats] -ERR " << f.payload << "\n";
        return;
    case FrameKind::Info:
    case FrameKind::Ok:
    case FrameKind::Pong:
        return;
    }
}

}  // namespace wp::nats
