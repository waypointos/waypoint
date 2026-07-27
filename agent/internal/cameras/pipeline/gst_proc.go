package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"

	"github.com/waypointos/waypoint/agent/internal/cameras/h264"
)

// gstProc launches `gst-launch-1.0` with the given pipeline args. Stdout is
// expected to be a raw Annex-B byte stream (fdsink fd=1). Each NAL is
// pushed onto out; the channel is closed when stdout EOFs or ctx is done.
type gstProc struct {
	cmd    *exec.Cmd
	out    chan []byte
	cancel context.CancelFunc
}

func startGst(ctx context.Context, args []string) (<-chan []byte, *gstProc, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, "gst-launch-1.0", append([]string{"-q"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, nil, err
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("start gst-launch: %w", err)
	}

	// Log stderr — useful when debugging missing plugins.
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			slog.Info(fmt.Sprintf("[gst] %s", sc.Text()))
		}
	}()

	out := make(chan []byte, 64)
	gp := &gstProc{cmd: cmd, out: out, cancel: cancel}
	go gp.pump(stdout)
	go func() {
		werr := cmd.Wait()
		// A pipeline that fails to construct (missing element, bad caps) exits
		// almost immediately. ctx.Err() distinguishes that from an intentional
		// Stop(), which kills the process via CommandContext.
		if werr != nil && ctx.Err() == nil {
			slog.Error(fmt.Sprintf("[gst] pipeline exited unexpectedly: %v", werr))
		}
		close(out)
	}()
	return out, gp, nil
}

func (g *gstProc) pump(r io.ReadCloser) {
	defer r.Close()
	nr := h264.NewReader(r)
	for {
		nal, err := nr.ReadNAL()
		if nal != nil && len(nal) > 0 {
			select {
			case g.out <- nal:
			default:
				// channel full: drop frame rather than stall encoder
			}
		}
		if err != nil {
			return
		}
	}
}

func (g *gstProc) Stop() {
	if g.cancel != nil {
		g.cancel()
	}
}
