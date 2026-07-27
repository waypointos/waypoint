// Command waypoint-dev brings up a simulated rover locally: the real agent and
// the real core in sim mode (behavioral STS3215 model, realtime virtual clock).
// A dashboard can attach to the agent's HTTP gateway just like a real rover.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/waypointos/waypoint/sim/internal/harness"
)

func main() {
	agentBin := flag.String("agent", "../bin/waypoint-agent", "path to a built waypoint-agent")
	coreBin := flag.String("core", "../core/build/src/waypoint-core", "path to a built waypoint-core")
	descriptor := flag.String("descriptor", "../protocol/platform/waypoint-rover.toml", "platform descriptor path")
	roverID := flag.String("rover-id", "sim-rover", "rover id")
	httpAddr := flag.String("http", ":8080", "agent HTTP listen address")
	flag.Parse()

	logDir, err := os.MkdirTemp("", "waypoint-dev-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "waypoint-dev: tempdir: %v\n", err)
		os.Exit(1)
	}

	r, err := harness.Launch(context.Background(), harness.Opts{
		AgentBin:       *agentBin,
		CoreBin:        *coreBin,
		DescriptorPath: *descriptor,
		RoverID:        *roverID,
		Controlled:     false, // realtime: a dashboard drives wall-clock telemetry
		HTTPAddr:       *httpAddr,
		LogDir:         logDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "waypoint-dev: launch: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("dev rover up: rover-id=%s dashboard=http://localhost%s\n", *roverID, *httpAddr)

	// Stream both children's captured logs to stdout, prefixed.
	go tailLog(filepath.Join(logDir, "agent.log"), "[agent]")
	go tailLog(filepath.Join(logDir, "core.log"), "[core]")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("\nwaypoint-dev: shutting down")
	r.Stop()
}

// tailLog follows a child log file and echoes new lines with a prefix.
func tailLog(path, prefix string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			fmt.Printf("%s %s", prefix, line)
		}
		if err == io.EOF {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if err != nil {
			return
		}
	}
}
