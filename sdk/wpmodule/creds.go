package wpmodule

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// loadCredsEnv parses the agent-written creds.env
// (WAYPOINT_NATS_USER=... / WAYPOINT_NATS_PASSWORD=...).
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
		return "", "", fmt.Errorf("creds %s: missing WAYPOINT_NATS_USER or WAYPOINT_NATS_PASSWORD", path)
	}
	return user, pass, nil
}
