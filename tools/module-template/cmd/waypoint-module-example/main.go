// Binary waypoint-module-example is a template Waypoint module. It connects to
// the rover's NATS bus, answers health probes, and publishes a periodic
// ExampleStats on waypoint.<rover-id>.module.example.stats.
//
// The agent starts it with three flags (see systemd/waypoint-module-example.service):
//
//	--config  path to config.toml   (agent-written, per rover)
//	--creds   path to creds.env      (agent-minted NATS user, mode 0600)
//	--rover   rover id               (used to build the concrete subject)
//
// Replace the publish loop with whatever your module actually does.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	natsgo "github.com/nats-io/nats.go"

	"github.com/waypointos/waypoint-module-example/internal/config"
	"github.com/waypointos/waypoint-module-example/internal/publisher"
)

func main() {
	configPath := flag.String("config", env("WAYPOINT_MODULE_CONFIG", ""), "path to config.toml")
	credsPath := flag.String("creds", env("WAYPOINT_MODULE_CREDS", ""), "path to creds.env")
	natsURL := flag.String("nats", env("WAYPOINT_NATS_URL", natsgo.DefaultURL), "nats URL")
	roverID := flag.String("rover", env("WAYPOINT_ROVER_ID", ""), "rover id")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if *configPath == "" || *credsPath == "" || *roverID == "" {
		slog.Error("waypoint-module-example: --config, --creds, --rover are required")
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error(fmt.Sprintf("config: %v", err))
		os.Exit(1)
	}

	user, pass, err := loadCredsEnv(*credsPath)
	if err != nil {
		slog.Error(fmt.Sprintf("creds: %v", err))
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	nc, err := natsgo.Connect(*natsURL,
		natsgo.UserInfo(user, pass),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(2*time.Second),
	)
	if err != nil {
		slog.Error(fmt.Sprintf("nats connect: %v", err))
		os.Exit(1)
	}
	defer nc.Drain()

	// Signal READY=1 as soon as NATS is connected. The unit is Type=notify with
	// a 90s default TimeoutStartSec, so never block readiness on slow external
	// work (a device warming up, a login). Gate health on that work instead.
	pub, err := publisher.New(nc, *roverID, cfg.Message)
	if err != nil {
		slog.Error(fmt.Sprintf("publisher: %v", err))
		os.Exit(1)
	}
	_, _ = daemon.SdNotify(false, daemon.SdNotifyReady)

	pub.SetInterval(cfg.Interval)
	pub.Run(ctx)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadCredsEnv reads WAYPOINT_NATS_USER / WAYPOINT_NATS_PASSWORD from the
// agent-minted creds.env (KEY=value lines).
func loadCredsEnv(path string) (string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	var user, pass string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "WAYPOINT_NATS_USER":
			user = v
		case "WAYPOINT_NATS_PASSWORD":
			pass = v
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if user == "" || pass == "" {
		return "", "", fmt.Errorf("creds: missing user or password")
	}
	return user, pass, nil
}
