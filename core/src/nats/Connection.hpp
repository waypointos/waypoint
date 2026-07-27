#pragma once

#include <atomic>
#include <cstdint>
#include <functional>
#include <map>
#include <memory>
#include <mutex>
#include <string>
#include <thread>

#include "nats/Frame.hpp"
#include "nats/Transport.hpp"

namespace wp::nats {

struct Message {
    std::string subject;
    std::string reply;
    std::string payload;
};

using MsgHandler = std::function<void(const Message&)>;

class Connection {
public:
    explicit Connection(std::unique_ptr<Transport> t);
    ~Connection();

    // Connect + send CONNECT line. Returns 0 on success.
    int start();

    // Subscribe to subject. cb is invoked on the read thread; keep it fast.
    // Returns the subscription id.
    std::uint64_t subscribe(const std::string& subject, MsgHandler cb);

    // Publish. Threadsafe. Returns 0 on success.
    int publish(const std::string& subject, const char* data, std::size_t n);
    int publishRequest(const std::string& subject, const std::string& reply,
                       const char* data, std::size_t n);

    // Synchronously shut down the read thread and close the transport.
    void stop();

private:
    void readLoop();
    // Blocks until transport_ is reconnected or running_ becomes false.
    // Caller must NOT hold writeMu_. On return the CONNECT line and all
    // SUB lines have been re-sent (under writeMu_) if reconnect succeeded.
    void reconnectLoop();
    void onFrame(const Frame& f);

    std::unique_ptr<Transport> transport_;
    Parser parser_;

    std::mutex writeMu_;
    std::mutex subsMu_;
    std::map<std::uint64_t, std::pair<std::string, MsgHandler>> subs_;
    std::atomic<std::uint64_t> nextSid_{1};
    std::atomic<bool> running_{false};
    std::thread reader_;
};

}  // namespace wp::nats
