package natsunix

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// tempSocketDir returns a short-path temp directory suitable for Unix domain
// sockets. macOS limits sun_path to 104 chars; the default t.TempDir() on
// darwin lands under /var/folders/... and routinely exceeds that. /tmp is
// short on every supported platform.
func tempSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "wp-relay-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestRelay_UnixClientCanPubSubViaTCPServer(t *testing.T) {
	s, _ := server.NewServer(&server.Options{Port: -1})
	go s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("server not ready")
	}

	sockPath := filepath.Join(tempSocketDir(t), "wp.sock")
	relay, err := Start(context.Background(), Config{
		SocketPath: sockPath,
		BackendURL: s.ClientURL(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Stop()

	// Wait for socket to exist.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// "Pretend to be C++ core": dial the unix socket, send NATS protocol by hand,
	// expect to receive a published message.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Send CONNECT + SUB + PING. The PONG response confirms the server has
	// processed the SUB — without that handshake the publish below can race
	// the SUB registration and the message gets dropped (NATS pub-sub is
	// fire-and-forget; nc.Flush only flushes the publisher's connection).
	_, _ = conn.Write([]byte("CONNECT {\"verbose\":false}\r\nSUB hello 1\r\nPING\r\n"))

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := readUntil(conn, "PONG"); err != nil {
		t.Fatalf("waiting for PONG after SUB: %v", err)
	}

	// Publish from a regular client via TCP — relay must forward it down to the unix client.
	nc, _ := nats.Connect(s.ClientURL())
	defer nc.Close()
	if err := nc.Publish("hello", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	_ = nc.Flush()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := readUntil(conn, "MSG hello 1"); err != nil {
		t.Fatalf("waiting for MSG: %v", err)
	}
}

// readUntil drains conn until needle appears in the accumulated stream or the
// read deadline fires.
func readUntil(conn net.Conn, needle string) error {
	var acc []byte
	buf := make([]byte, 4096)
	for {
		if contains(acc, needle) {
			return nil
		}
		n, err := conn.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
		}
		if err != nil {
			if contains(acc, needle) {
				return nil
			}
			return fmt.Errorf("%w (acc=%q)", err, string(acc))
		}
	}
}

func contains(haystack []byte, needle string) bool {
	return len(haystack) >= len(needle) &&
		string(haystack)[:len(haystack)] != "" &&
		bytesIndex(haystack, []byte(needle)) >= 0
}

func bytesIndex(h, n []byte) int {
	for i := 0; i+len(n) <= len(h); i++ {
		match := true
		for j := range n {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
