package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ModuleProc is a module binary running against a launched Rover.
type ModuleProc struct {
	cmd     *exec.Cmd
	log     *os.File
	LogPath string
}

// LaunchModule starts a module binary against the rover's NATS with the
// standard module env contract. Dev NATS is open, so no creds are minted;
// the module connects as the default user. extraEnv entries are KEY=VALUE.
func (r *Rover) LaunchModule(binPath, moduleID string, extraEnv ...string) (*ModuleProc, error) {
	logDir := r.Opts.LogDir
	if logDir == "" {
		logDir = os.TempDir()
	}
	logPath := filepath.Join(logDir, "module-"+moduleID+".log")
	logF, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("harness: module log: %w", err)
	}
	cmd := exec.Command(binPath)
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.Env = append(os.Environ(),
		"WAYPOINT_NATS_URL=nats://127.0.0.1:4222",
		"WAYPOINT_ROVER_ID="+r.Opts.RoverID,
		"WAYPOINT_MODULE_ID="+moduleID,
		"WAYPOINT_MODULE_CREDS=", // dev: plain connect
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	if err := cmd.Start(); err != nil {
		logF.Close()
		return nil, fmt.Errorf("harness: start module %s: %w", moduleID, err)
	}
	return &ModuleProc{cmd: cmd, log: logF, LogPath: logPath}, nil
}

// Stop terminates the module: SIGTERM, 2 s grace, SIGKILL.
func (p *ModuleProc) Stop() {
	if p == nil {
		return
	}
	stopProc(p.cmd)
	p.cmd = nil
	if p.log != nil {
		_ = p.log.Close()
		p.log = nil
	}
}
