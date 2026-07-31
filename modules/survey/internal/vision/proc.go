package vision

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/waypointos/waypoint/modules/survey/internal/mission"
)

// Silence beyond this means the camera pipeline is down; nav then runs on
// odometry alone.
const silenceFault = 3 * time.Second

const restartBackoff = time.Second

// Handler receives the reduced detections of one frame. Called from the
// runner goroutine.
type Handler func(dets []mission.Detection, now time.Time)

// Proc supervises the Python detector child: spawn, parse stdout JSON
// lines, restart on exit, and flag prolonged silence as a camera fault.
type Proc struct {
	Script      string
	Device      string
	Width       int
	Height      int
	HfovDeg     float64
	MarkerSizeM float64

	lastLine atomic.Int64 // unix nanos of the last stdout line
	faulted  atomic.Bool

	stdinMu sync.Mutex
	stdin   io.WriteCloser // live child's stdin, nil between runs
}

// Fault reports whether the detector is currently considered down.
func (p *Proc) Fault() bool { return p.faulted.Load() }

// Snap asks the running child to save its most recent frame as a JPEG at
// path (the "SNAP <path>" stdin line protocol).
func (p *Proc) Snap(path string) error {
	p.stdinMu.Lock()
	defer p.stdinMu.Unlock()
	if p.stdin == nil {
		return fmt.Errorf("vision: detector not running")
	}
	_, err := io.WriteString(p.stdin, "SNAP "+path+"\n")
	return err
}

// Run blocks until ctx is done, restarting the child as needed.
func (p *Proc) Run(ctx context.Context, h Handler) {
	p.lastLine.Store(time.Now().UnixNano())
	go p.watch(ctx)
	for ctx.Err() == nil {
		if err := p.runOnce(ctx, h); err != nil && ctx.Err() == nil {
			slog.Warn("vision: detector exited", "err", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(restartBackoff):
		}
	}
}

func (p *Proc) runOnce(ctx context.Context, h Handler) error {
	cmd := exec.CommandContext(ctx, "python3", p.Script,
		"--device", p.Device,
		"--width", strconv.Itoa(p.Width),
		"--height", strconv.Itoa(p.Height),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", p.Script, err)
	}
	p.stdinMu.Lock()
	p.stdin = stdin
	p.stdinMu.Unlock()
	defer func() {
		p.stdinMu.Lock()
		p.stdin = nil
		p.stdinMu.Unlock()
	}()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			slog.Warn("vision: detect.py", "msg", sc.Text())
		}
	}()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		now := time.Now()
		p.lastLine.Store(now.UnixNano())
		f, err := DecodeFrame(sc.Bytes())
		if err != nil {
			slog.Warn("vision: bad frame line", "err", err)
			continue
		}
		if dets := f.Reduce(p.HfovDeg, p.MarkerSizeM); len(dets) > 0 {
			h(dets, now)
		}
	}
	return cmd.Wait()
}

func (p *Proc) watch(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			silent := time.Since(time.Unix(0, p.lastLine.Load())) > silenceFault
			if silent && !p.faulted.Swap(true) {
				slog.Error("vision: camera fault, no frames for 3s; navigating on odometry only")
			}
			if !silent && p.faulted.Swap(false) {
				slog.Info("vision: camera recovered")
			}
		}
	}
}
