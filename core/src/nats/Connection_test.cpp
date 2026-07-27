#include <gtest/gtest.h>

#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

#include <algorithm>
#include <chrono>
#include <condition_variable>
#include <cstdlib>
#include <cstring>
#include <filesystem>
#include <mutex>
#include <string>
#include <thread>

#include "nats/Connection.hpp"
#include "nats/UnixTransport.hpp"

using namespace wp::nats;
using namespace std::chrono_literals;

// ---------------------------------------------------------------------------
// Minimal in-process Unix socket server for reconnect testing.
// Accepts multiple sequential connections and records the text lines received
// on each. Each accepted connection is "served" until the client closes it (or
// the server calls close(2) on its side), then the server moves on to the next
// accept().
// ---------------------------------------------------------------------------
struct TestServer {
    std::string path;
    int listenFd = -1;

    // All lines received, grouped by connection index.
    // lines[0] = lines from first accepted connection, etc.
    std::mutex mu;
    std::condition_variable cv;
    std::vector<std::vector<std::string>> lines;  // lines[connIdx][lineIdx]

    bool done = false;  // set true when expected connections have been served
    std::string err;    // set to non-empty on setup error

    // Start listening on path. Returns true on success.
    bool listen() {
        listenFd = ::socket(AF_UNIX, SOCK_STREAM, 0);
        if (listenFd < 0) { err = "socket"; return false; }

        sockaddr_un addr{};
        addr.sun_family = AF_UNIX;
        std::strncpy(addr.sun_path, path.c_str(), sizeof(addr.sun_path) - 1);
        ::unlink(path.c_str());  // remove stale socket

        if (::bind(listenFd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) < 0) {
            err = "bind"; return false;
        }
        if (::listen(listenFd, 4) < 0) { err = "listen"; return false; }
        return true;
    }

    // Accept exactly `count` connections. For each, read complete text lines
    // until `minLines` distinct non-empty lines have been received, then close
    // the server side of the connection (triggering EOF on the client). Records
    // all received lines. When `count` connections have been served, signals
    // `done`.
    void serveN(int count, int minLines = 2) {
        for (int i = 0; i < count; ++i) {
            int clientFd = ::accept(listenFd, nullptr, nullptr);
            if (clientFd < 0) {
                std::lock_guard<std::mutex> lk(mu);
                err = "accept";
                done = true;
                cv.notify_all();
                return;
            }

            std::vector<std::string> connLines;
            std::string partial;
            char buf[512];

            // Read until we have collected at least minLines complete lines or
            // the client closes its end.
            while (static_cast<int>(connLines.size()) < minLines) {
                ssize_t n = ::read(clientFd, buf, sizeof(buf));
                if (n <= 0) break;  // client closed unexpectedly
                partial.append(buf, static_cast<std::size_t>(n));
                // Extract complete lines.
                std::size_t pos = 0;
                while (true) {
                    auto nl = partial.find('\n', pos);
                    if (nl == std::string::npos) break;
                    std::string line = partial.substr(pos, nl - pos);
                    // Strip trailing \r if present (NATS uses \r\n).
                    if (!line.empty() && line.back() == '\r') line.pop_back();
                    if (!line.empty()) connLines.push_back(line);
                    pos = nl + 1;
                }
                partial = partial.substr(pos);
            }
            // Close our side — the client's next read() will return 0,
            // triggering the reconnect path.
            ::close(clientFd);

            {
                std::lock_guard<std::mutex> lk(mu);
                lines.push_back(std::move(connLines));
            }
        }

        std::lock_guard<std::mutex> lk(mu);
        done = true;
        cv.notify_all();
    }

    void stopListening() {
        if (listenFd >= 0) {
            ::close(listenFd);
            listenFd = -1;
        }
        ::unlink(path.c_str());
    }

    // Returns true if any line in the given connection's lines contains `needle`.
    bool hasLine(std::size_t connIdx, const std::string& needle) const {
        if (connIdx >= lines.size()) return false;
        for (auto& l : lines[connIdx]) {
            if (l.find(needle) != std::string::npos) return true;
        }
        return false;
    }
};

// Verify that Connection reconnects after the transport is closed and re-sends
// the CONNECT handshake plus all active SUBs on the new connection.
TEST(Connection, ReconnectsAfterTransportClose) {
    // Build a unique temp socket path.
    auto tmpDir = std::filesystem::temp_directory_path();
    std::string sockPath = (tmpDir / "wp_reconnect_test.sock").string();

    TestServer server;
    server.path = sockPath;
    ASSERT_TRUE(server.listen()) << "listen failed: " << server.err;

    // Server thread: accept exactly 2 connections, record lines, signal done.
    std::thread serverThread([&] { server.serveN(2); });

    // Build the Connection using a UnixTransport pointing at our test server.
    auto transport = std::make_unique<UnixTransport>(sockPath);
    Connection conn(std::move(transport));
    ASSERT_EQ(conn.start(), 0);

    // Subscribe before the first close so the SUB line appears on connection 0
    // and must be re-sent on connection 1 after reconnect.
    conn.subscribe("waypoint.test.foo", [](const Message&) {});

    // Wait for server to finish serving both connections (5 second timeout).
    {
        std::unique_lock<std::mutex> lk(server.mu);
        bool ok = server.cv.wait_for(lk, 5s, [&] { return server.done; });
        ASSERT_TRUE(ok) << "timed out waiting for two connections";
        ASSERT_TRUE(server.err.empty()) << "server error: " << server.err;
    }

    conn.stop();
    serverThread.join();
    server.stopListening();

    // Both connections must carry a CONNECT line.
    EXPECT_TRUE(server.hasLine(0, "CONNECT")) << "connection 0 missing CONNECT";
    EXPECT_TRUE(server.hasLine(1, "CONNECT")) << "connection 1 missing CONNECT";

    // Both connections must carry a SUB line for our subject.
    EXPECT_TRUE(server.hasLine(0, "SUB waypoint.test.foo"))
        << "connection 0 missing SUB";
    EXPECT_TRUE(server.hasLine(1, "SUB waypoint.test.foo"))
        << "connection 1 missing SUB re-sent after reconnect";
}

// ---------------------------------------------------------------------------
// This test requires the agent's natsunix relay to be running and the env var
// WAYPOINT_TEST_SOCKET to point at the socket path. Skip otherwise — keeps
// `make core-test` green without external setup, but exercises real wire
// behavior when the user is running the agent.
TEST(Connection, RoundTripAgainstAgent) {
    const char* sockEnv = std::getenv("WAYPOINT_TEST_SOCKET");
    if (!sockEnv || !std::filesystem::exists(sockEnv)) {
        GTEST_SKIP() << "set WAYPOINT_TEST_SOCKET to /tmp/waypoint-nats.sock with agent running";
    }

    auto transport = std::make_unique<UnixTransport>(sockEnv);
    Connection conn(std::move(transport));
    ASSERT_EQ(conn.start(), 0);

    std::mutex mu;
    std::condition_variable cv;
    std::string got;
    conn.subscribe("test.echo", [&](const Message& m) {
        std::lock_guard<std::mutex> lk(mu);
        got = m.payload;
        cv.notify_one();
    });
    // Give the SUB line time to be processed by the server.
    std::this_thread::sleep_for(50ms);

    const char* body = "hello-core";
    ASSERT_EQ(conn.publish("test.echo", body, std::strlen(body)), 0);

    std::unique_lock<std::mutex> lk(mu);
    ASSERT_TRUE(cv.wait_for(lk, 3s, [&] { return !got.empty(); }));
    EXPECT_EQ(got, "hello-core");
    conn.stop();
}
