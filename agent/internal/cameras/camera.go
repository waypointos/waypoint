// Package cameras owns the per-camera GStreamer pipeline, the pion
// PeerConnection fan-out, and the WHEP HTTP endpoints. One Camera per
// physical capture device. One Manager per agent.
package cameras

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/waypointos/waypoint/agent/internal/cameras/pipeline"
)

// Pipeline restart backoff bounds (vars so tests can shrink them). A pipeline
// that exits is restarted after pipelineRestartMin, doubling up to
// pipelineRestartMax; a run lasting pipelineHealthyRun resets the delay.
var (
	pipelineRestartMin = time.Second
	pipelineRestartMax = 15 * time.Second
	pipelineHealthyRun = 30 * time.Second
)

type Camera struct {
	Name     string
	pipeline pipeline.Pipeline
	iceCfg   func() []webrtc.ICEServer

	mu   sync.Mutex
	subs map[string]*subscriber

	frameMu   sync.RWMutex
	curTracks []*webrtc.TrackLocalStaticSample
	taps      map[int]func(au []byte, keyframe bool)
	nextTap   int
	startedAt time.Time
	stopFn    context.CancelFunc
}

type subscriber struct {
	id    string
	pc    *webrtc.PeerConnection
	track *webrtc.TrackLocalStaticSample
}

func New(name string, p pipeline.Pipeline, iceCfg func() []webrtc.ICEServer) (*Camera, error) {
	if name == "" {
		return nil, errors.New("name required")
	}
	return &Camera{
		Name:     name,
		pipeline: p,
		iceCfg:   iceCfg,
		subs:     map[string]*subscriber{},
	}, nil
}

// Start launches the pipeline and begins a goroutine that fans NAL samples
// out to every active subscriber track. Idempotent.
func (c *Camera) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopFn != nil {
		return nil
	}
	subCtx, cancel := context.WithCancel(ctx)
	c.stopFn = cancel
	c.startedAt = time.Now()

	// frames outlives any single pipeline run, so a pipeline restart (USB
	// hiccup, transient device error) is invisible to fanout and subscribers.
	frames := make(chan []byte, 64)
	go c.supervise(subCtx, frames)
	go c.fanout(subCtx, frames)
	return nil
}

// supervise runs the pipeline and forwards its NAL units into out, restarting
// it with exponential backoff whenever it exits. A camera that hiccups or
// briefly disappears recovers on its own instead of going dark until the agent
// restarts. A run lasting pipelineHealthyRun resets the backoff; one that dies
// immediately (missing device/element) backs off to the cap to avoid spinning.
func (c *Camera) supervise(ctx context.Context, out chan<- []byte) {
	defer close(out)
	backoff := pipelineRestartMin
	for {
		if ctx.Err() != nil {
			return
		}
		runStart := time.Now()
		src, err := c.pipeline.Start(ctx)
		if err != nil {
			slog.Warn(fmt.Sprintf("camera %s: pipeline start: %v", c.Name, err))
		} else {
			for nal := range src {
				select {
				case out <- nal:
				default: // fanout busy / no subscribers: drop rather than stall
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		if time.Since(runStart) >= pipelineHealthyRun {
			backoff = pipelineRestartMin
		}
		slog.Warn(fmt.Sprintf("camera %s: pipeline exited, restarting in %s", c.Name, backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < pipelineRestartMax {
			backoff *= 2
			if backoff > pipelineRestartMax {
				backoff = pipelineRestartMax
			}
		}
	}
}

func (c *Camera) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopFn != nil {
		c.stopFn()
		c.stopFn = nil
	}
	c.pipeline.Stop()
	for id, s := range c.subs {
		_ = s.pc.Close()
		delete(c.subs, id)
	}
}

// AddTap registers a consumer of complete Annex-B access units alongside the
// WebRTC tracks. The callback runs on the fanout goroutine: it must not block
// (copy and hand off).
func (c *Camera) AddTap(fn func(au []byte, keyframe bool)) (remove func()) {
	c.frameMu.Lock()
	defer c.frameMu.Unlock()
	if c.taps == nil {
		c.taps = map[int]func([]byte, bool){}
	}
	id := c.nextTap
	c.nextTap++
	c.taps[id] = fn
	return func() {
		c.frameMu.Lock()
		delete(c.taps, id)
		c.frameMu.Unlock()
	}
}

// nalTypeAUD is the H.264 Access Unit Delimiter NAL. h264parse inserts one
// before every access unit, giving us a clean per-frame boundary.
const nalTypeAUD = 9

// nalTypeIDR is the coded slice of an IDR picture: its presence in an access
// unit makes that AU a keyframe (the decodable entry point for the stream).
const nalTypeIDR = 5

// fanout consumes NAL units from the pipeline, groups them into Access Units
// (one AU == one video frame == one pion Sample), reconstructs the Annex-B
// byte stream, and writes the AU to every subscriber track.
//
// Why per-AU and not per-NAL: pion's TrackLocalStaticSample.WriteSample
// advances the RTP timestamp by Sample.Duration on every call. If we wrote
// one Sample per NAL (and a keyframe AU is AUD+SPS+PPS+SEI+IDR = 5 NALs),
// timestamps would drift ~5x wall-clock and the browser would render black.
// One Sample per AU keeps the timestamp scale honest.
func (c *Camera) fanout(ctx context.Context, frames <-chan []byte) {
	const fps = 30
	const startCode4 = "\x00\x00\x00\x01"
	frameDur := time.Second / time.Duration(fps)

	var au []byte     // Annex-B byte stream of NALs accumulated for the current AU
	var keyframe bool // set when the current AU carries an IDR slice

	flush := func() {
		if len(au) == 0 {
			return
		}
		c.frameMu.RLock()
		tracks := append([]*webrtc.TrackLocalStaticSample(nil), c.curTracks...)
		taps := make([]func([]byte, bool), 0, len(c.taps))
		for _, fn := range c.taps {
			taps = append(taps, fn)
		}
		c.frameMu.RUnlock()
		for _, t := range tracks {
			_ = t.WriteSample(media.Sample{Data: au, Duration: frameDur})
		}
		for _, fn := range taps {
			fn(au, keyframe)
		}
		keyframe = false
		// Drop the slice rather than truncating it. Pion's H.264 packetizer
		// is documented as synchronous within WriteSample, but with multiple
		// concurrent tracks subscribed to the same Camera there have been
		// reports of the second track decoding nothing — most plausible
		// explanation is a slice reference outliving the WriteSample call.
		// Letting GC own the slice and forcing a fresh allocation on the
		// next AU is the cheap defensive move.
		au = nil
	}

	for {
		select {
		case <-ctx.Done():
			return
		case nal, ok := <-frames:
			if !ok {
				flush()
				return
			}
			if len(nal) == 0 {
				continue
			}
			// AUD (NAL type 9) marks the start of a new access unit; flush
			// whatever we accumulated for the previous AU before appending.
			if nal[0]&0x1f == nalTypeAUD {
				flush()
			}
			if nal[0]&0x1f == nalTypeIDR {
				keyframe = true
			}
			au = append(au, startCode4...)
			au = append(au, nal...)
		}
	}
}

func (c *Camera) NewSubscriber(ctx context.Context, sessionID string, offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: c.iceCfg()})
	if err != nil {
		return nil, err
	}
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
		"video", "waypoint-"+c.Name,
	)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if _, err := pc.AddTrack(track); err != nil {
		_ = pc.Close()
		return nil, err
	}

	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		return nil, err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return nil, err
	}
	<-webrtc.GatheringCompletePromise(pc)

	c.frameMu.Lock()
	c.curTracks = append(c.curTracks, track)
	c.frameMu.Unlock()

	c.mu.Lock()
	c.subs[sessionID] = &subscriber{id: sessionID, pc: pc, track: track}
	c.mu.Unlock()

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed || s == webrtc.PeerConnectionStateDisconnected {
			c.removeSubscriber(sessionID)
		}
	})

	return pc.LocalDescription(), nil
}

func (c *Camera) removeSubscriber(id string) {
	c.mu.Lock()
	sub, ok := c.subs[id]
	if ok {
		delete(c.subs, id)
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	c.frameMu.Lock()
	for i, t := range c.curTracks {
		if t == sub.track {
			c.curTracks = append(c.curTracks[:i], c.curTracks[i+1:]...)
			break
		}
	}
	c.frameMu.Unlock()
	_ = sub.pc.Close()
}

func (c *Camera) EndSubscriber(id string) { c.removeSubscriber(id) }
