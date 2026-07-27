package harness

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// natsSocketPath is the dev (non-Linux) unix socket both agent and core use.
// Mirrors agent main.go and core Args.cpp.
const natsSocketPath = "/tmp/waypoint-nats.sock"

type Opts struct {
	AgentBin       string // path to a built waypoint-agent
	CoreBin        string // path to a built waypoint-core
	DescriptorPath string // canonical platform descriptor
	RoverID        string // default "sim-rover"
	Controlled     bool   // pass --sim-time=controlled to core
	HTTPAddr       string // agent HTTP listen address; default ":8080"
	LogDir         string // child stdout/stderr capture (caller-provided temp dir)
}

type Rover struct {
	Opts  Opts
	NC    *nats.Conn
	Trace *Trace

	// EpisodesDir is the per-run recorder output dir so tests can inspect the
	// MCAP and sidecar artifacts.
	EpisodesDir string

	agent        *exec.Cmd
	core         *exec.Cmd
	agentLog     *os.File
	coreLog      *os.File
	agentLogPath string
	coreLogPath  string
}

func (o *Opts) applyDefaults() {
	if o.RoverID == "" {
		o.RoverID = "sim-rover"
	}
	if o.HTTPAddr == "" {
		o.HTTPAddr = ":8080"
	}
}

// Launch starts the real agent and the real core in sim mode, waits until both
// are healthy (NATS socket up + rpc.sim_info answers), and returns a connected
// Rover. Always call Stop, including on test failure paths.
func Launch(ctx context.Context, o Opts) (*Rover, error) {
	o.applyDefaults()
	r := &Rover{Opts: o}

	if o.AgentBin == "" || o.CoreBin == "" {
		return nil, fmt.Errorf("harness: AgentBin and CoreBin are required")
	}

	// Resolve to an absolute path: the agent deny-list override is read from a
	// child process that may run with a different cwd than the harness.
	absDesc, err := filepath.Abs(o.DescriptorPath)
	if err != nil {
		return nil, fmt.Errorf("harness: descriptor path: %w", err)
	}

	logDir := o.LogDir
	if logDir == "" {
		logDir = os.TempDir()
	}

	// Identity TOML: rover id + a bootstrap admin token (required by the agent
	// config loader). No [proxy] block: NATS runs local-only.
	idPath := filepath.Join(logDir, "identity.toml")
	idContent := fmt.Sprintf("[rover]\nid = %q\nname = \"Sim Rover\"\n\n[local]\nbootstrap_admin_token = \"sim-do-not-use-in-prod\"\n", o.RoverID)
	if err := os.WriteFile(idPath, []byte(idContent), 0600); err != nil {
		return nil, fmt.Errorf("harness: write identity: %w", err)
	}

	// Start fresh: a stale socket from a prior crashed run blocks the relay.
	_ = os.Remove(natsSocketPath)

	// The embedded NATS binds a fixed TCP port; back-to-back launches (the
	// conformance suite) must wait for the prior agent to release it.
	if err := waitForPortFree("127.0.0.1:4222", 10*time.Second); err != nil {
		return nil, fmt.Errorf("harness: nats port busy: %w", err)
	}

	r.agentLogPath = filepath.Join(logDir, "agent.log")
	r.coreLogPath = filepath.Join(logDir, "core.log")
	r.EpisodesDir = filepath.Join(logDir, "episodes")

	// Start the agent. WAYPOINT_SKIP_VERIFY marks a dev image (no module
	// signature checks). The agent serves NATS on TCP :4222 and the dev socket.
	agentLog, err := os.Create(r.agentLogPath)
	if err != nil {
		return nil, fmt.Errorf("harness: create agent log: %w", err)
	}
	r.agentLog = agentLog
	r.agent = exec.Command(o.AgentBin, "-identity", idPath, "-http", o.HTTPAddr)
	r.agent.Stdout = agentLog
	r.agent.Stderr = agentLog
	r.agent.Env = append(os.Environ(),
		"WAYPOINT_SKIP_VERIFY=1",
		"WAYPOINT_PLATFORM_DESCRIPTOR="+absDesc,
		"WAYPOINT_NATS_SOCKET="+natsSocketPath,
		"WAYPOINT_MODULE_INSTALL_DIR="+filepath.Join(logDir, "modules-install"),
		"WAYPOINT_MODULES_CONFIG="+filepath.Join(logDir, "modules.toml"),
		"WAYPOINT_MODULE_RUNTIME_DIR="+filepath.Join(logDir, "modules-run"),
		// Off-rover, /data/waypoint does not exist; redirect the persistence
		// stores (fatal at boot otherwise) into the caller's temp dir.
		"WAYPOINT_AUDIT_DB="+filepath.Join(logDir, "audit.db"),
		"WAYPOINT_ALERTS_DB="+filepath.Join(logDir, "alerts.db"),
		"WAYPOINT_LOCAL_ACCOUNT_SEED="+filepath.Join(logDir, "local-account.nk"),
		"WAYPOINT_EPISODES_DIR="+r.EpisodesDir,
	)
	if err := r.agent.Start(); err != nil {
		r.closeLogs()
		return nil, fmt.Errorf("harness: start agent: %w", err)
	}

	// Honor a cancelled context before the slow waits below.
	if err := ctx.Err(); err != nil {
		r.Stop()
		return nil, err
	}

	// Wait for the NATS dev socket and the TCP listener.
	if err := waitForSocket(natsSocketPath, 10*time.Second); err != nil {
		r.Stop()
		return nil, fmt.Errorf("harness: agent socket not ready: %w (agent log:\n%s)", err, r.tail(r.agentLogPath))
	}

	nc, err := connectNATS("nats://127.0.0.1:4222", 10*time.Second)
	if err != nil {
		r.Stop()
		return nil, fmt.Errorf("harness: nats connect: %w (agent log:\n%s)", err, r.tail(r.agentLogPath))
	}
	r.NC = nc

	r.Trace = newTrace(o.RoverID)
	if _, err := nc.Subscribe("waypoint."+o.RoverID+".>", func(m *nats.Msg) {
		r.Trace.record(m.Subject, append([]byte(nil), m.Data...))
	}); err != nil {
		r.Stop()
		return nil, fmt.Errorf("harness: subscribe trace: %w", err)
	}

	// Start core in sim mode against the same socket.
	coreLog, err := os.Create(r.coreLogPath)
	if err != nil {
		r.Stop()
		return nil, fmt.Errorf("harness: create core log: %w", err)
	}
	r.coreLog = coreLog
	coreArgs := []string{
		"--descriptor", o.DescriptorPath,
		"--servo-mock",
		"--rover-id", o.RoverID,
		"--socket", natsSocketPath,
	}
	if o.Controlled {
		coreArgs = append(coreArgs, "--sim-time", "controlled")
	}
	r.core = exec.Command(o.CoreBin, coreArgs...)
	r.core.Stdout = coreLog
	r.core.Stderr = coreLog
	if err := r.core.Start(); err != nil {
		r.Stop()
		return nil, fmt.Errorf("harness: start core: %w", err)
	}

	// Health: rpc.sim_info answers once the sim driver's NATS surface is up.
	if err := r.waitForSimInfo(10 * time.Second); err != nil {
		r.Stop()
		return nil, fmt.Errorf("harness: sim_info not ready: %w (core log:\n%s)", err, r.tail(r.coreLogPath))
	}

	return r, nil
}

func (r *Rover) waitForSimInfo(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := r.SimInfo(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timeout")
	}
	return lastErr
}

// Stop terminates core then agent: SIGTERM, 2 s grace, SIGKILL. Controlled mode
// blocks in the virtual clock, so SIGKILL is the documented fallback.
func (r *Rover) Stop() {
	if r.NC != nil {
		r.NC.Close()
		r.NC = nil
	}
	stopProc(r.core)
	stopProc(r.agent)
	r.core = nil
	r.agent = nil
	r.closeLogs()
}

func stopProc(c *exec.Cmd) {
	if c == nil || c.Process == nil {
		return
	}
	_ = c.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_, _ = c.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = c.Process.Kill()
		<-done
	}
}

func (r *Rover) closeLogs() {
	if r.agentLog != nil {
		_ = r.agentLog.Close()
		r.agentLog = nil
	}
	if r.coreLog != nil {
		_ = r.coreLog.Close()
		r.coreLog = nil
	}
}

func (r *Rover) tail(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.Join(lines, "\n")
}

// Advance grants virtual time in controlled mode, chunked into 200 ms slices
// with a heartbeat published between slices so the core watchdog stays fed.
func (r *Rover) Advance(d time.Duration) error {
	return r.advance(d, true)
}

// AdvanceWithoutHeartbeats grants virtual time without feeding the watchdog,
// for tests that exercise heartbeat-loss failsafes.
func (r *Rover) AdvanceWithoutHeartbeats(d time.Duration) error {
	return r.advance(d, false)
}

func (r *Rover) advance(d time.Duration, heartbeats bool) error {
	const chunk = 200 * time.Millisecond
	// Flush so every fire-and-forget command published before this advance
	// (drive, mode, scenario) reaches the server, and therefore core, ahead of
	// the sim_advance grant. Without this the grant can be delivered to core
	// before a just-published command, making the trace nondeterministic.
	if err := r.NC.Flush(); err != nil {
		return err
	}
	remaining := d
	for remaining > 0 {
		step := chunk
		if remaining < step {
			step = remaining
		}
		if heartbeats {
			if err := r.Heartbeat(); err != nil {
				return err
			}
		}
		req := &waypointv1.SimAdvanceRequest{AdvanceMs: uint64(step.Milliseconds())}
		payload, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		// The reply is the barrier: the blocked reader thread guarantees the
		// loop consumed the whole grant before responding.
		msg, err := r.NC.Request(r.subj("rpc.sim_advance"), payload, 5*time.Second)
		if err != nil {
			return fmt.Errorf("sim_advance: %w", err)
		}
		var resp waypointv1.SimAdvanceResponse
		if err := proto.Unmarshal(msg.Data, &resp); err != nil {
			return err
		}
		if resp.Error != nil && *resp.Error != "" {
			return fmt.Errorf("sim_advance rejected: %s", *resp.Error)
		}
		// Drain: a round-trip flush makes the server deliver every telemetry
		// message it has already queued for this connection (published by core
		// during the grant) before the next step or the caller's assertions, so
		// the trace reflects the whole window.
		if err := r.NC.Flush(); err != nil {
			return err
		}
		remaining -= step
	}
	return nil
}

// Scenario publishes cmd.sim_scenario.
func (r *Rover) Scenario(s *waypointv1.SimScenario) error {
	payload, err := proto.Marshal(s)
	if err != nil {
		return err
	}
	return r.NC.Publish(r.subj("cmd.sim_scenario"), payload)
}

// Heartbeat publishes infra.heartbeat (empty payload).
func (r *Rover) Heartbeat() error {
	return r.NC.Publish(r.subj("infra.heartbeat"), nil)
}

// Estop fires rpc.estop (empty payload). Fire-and-forget, matching the
// dashboard convention; core ignores the request body.
func (r *Rover) Estop() error {
	return r.NC.Publish(r.subj("rpc.estop"), nil)
}

// Recover fires rpc.recover (empty payload). Fire-and-forget.
func (r *Rover) Recover() error {
	return r.NC.Publish(r.subj("rpc.recover"), nil)
}

// SetModeManual publishes a ModeEvent{to=MANUAL} to rpc.set_mode, mirroring the
// dashboard's fire-and-forget convention (core reuses ModeEvent; reads `to`).
// A heartbeat goes first: core refuses Manual without a live heartbeat, and
// same-connection ordering guarantees core marks the heartbeat fresh before it
// processes the mode request.
func (r *Rover) SetModeManual() error {
	if err := r.Heartbeat(); err != nil {
		return err
	}
	ev := &waypointv1.ModeEvent{To: waypointv1.Mode_MODE_MANUAL}
	payload, err := proto.Marshal(ev)
	if err != nil {
		return err
	}
	return r.NC.Publish(r.subj("rpc.set_mode"), payload)
}

// SetModeSafe publishes a ModeEvent{to=SAFE} to rpc.set_mode.
func (r *Rover) SetModeSafe() error {
	ev := &waypointv1.ModeEvent{To: waypointv1.Mode_MODE_SAFE}
	payload, err := proto.Marshal(ev)
	if err != nil {
		return err
	}
	return r.NC.Publish(r.subj("rpc.set_mode"), payload)
}

// Drive publishes a DriveCommand on cmd.drive.
func (r *Rover) Drive(vx, wz float64) error {
	cmd := &waypointv1.DriveCommand{BodyVxMps: &vx, YawRateRadps: &wz}
	payload, err := proto.Marshal(cmd)
	if err != nil {
		return err
	}
	return r.NC.Publish(r.subj("cmd.drive"), payload)
}

// ServoRead requests rpc.servo_read for one servo id.
func (r *Rover) ServoRead(id uint32) (*waypointv1.ServoState, error) {
	req := &waypointv1.ServoReadRequest{ServoId: id}
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	msg, err := r.NC.Request(r.subj("rpc.servo_read"), payload, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("servo_read: %w", err)
	}
	var st waypointv1.ServoState
	if err := proto.Unmarshal(msg.Data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// ServoControl publishes a ServoControl op on cmd.servo.
func (r *Rover) ServoControl(c *waypointv1.ServoControl) error {
	payload, err := proto.Marshal(c)
	if err != nil {
		return err
	}
	return r.NC.Publish(r.subj("cmd.servo"), payload)
}

// RecorderStart begins an episode via rpc.recorder_start.
func (r *Rover) RecorderStart(label string) (*waypointv1.RecorderStartResponse, error) {
	req, err := proto.Marshal(&waypointv1.RecorderStartRequest{TaskLabel: label})
	if err != nil {
		return nil, err
	}
	msg, err := r.NC.Request(r.subj("rpc.recorder_start"), req, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("recorder_start: %w", err)
	}
	var resp waypointv1.RecorderStartResponse
	if err := proto.Unmarshal(msg.Data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RecorderStop finalizes the active episode via rpc.recorder_stop.
func (r *Rover) RecorderStop(success bool, notes string) (*waypointv1.RecorderStopResponse, error) {
	req, err := proto.Marshal(&waypointv1.RecorderStopRequest{Success: success, Notes: notes})
	if err != nil {
		return nil, err
	}
	msg, err := r.NC.Request(r.subj("rpc.recorder_stop"), req, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("recorder_stop: %w", err)
	}
	var resp waypointv1.RecorderStopResponse
	if err := proto.Unmarshal(msg.Data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SimInfo requests rpc.sim_info.
func (r *Rover) SimInfo() (*waypointv1.SimInfo, error) {
	req := &waypointv1.SimInfoRequest{}
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	msg, err := r.NC.Request(r.subj("rpc.sim_info"), payload, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("sim_info: %w", err)
	}
	var info waypointv1.SimInfo
	if err := proto.Unmarshal(msg.Data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (r *Rover) subj(leaf string) string {
	return "waypoint." + r.Opts.RoverID + "." + leaf
}

// waitForPortFree blocks until a TCP listen on addr succeeds (the port is free)
// or the timeout elapses. Used between conformance launches that share :4222.
func waitForPortFree(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("port %s still in use after %s", addr, timeout)
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("socket %s not present after %s", path, timeout)
}

func connectNATS(url string, timeout time.Duration) (*nats.Conn, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		nc, err := nats.Connect(url, nats.Timeout(1*time.Second))
		if err == nil {
			return nc, nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	return nil, lastErr
}
