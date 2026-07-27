// Package recorder captures declared observation, action, and video streams
// into per-episode MCAP files.
package recorder

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

const defaultMinFree = 500 << 20 // 500 MiB

// CameraTaps is implemented by cameras.Manager.
type CameraTaps interface {
	Tap(fn func(camID string, au []byte, keyframe bool)) (remove func())
}

type Config struct {
	NC         *nats.Conn
	RoverID    string
	Dir        string
	PlatformID string
	// Specs returns the recording set; called at every episode start so a
	// module installed mid-session is included in the next episode.
	Specs        func() []StreamSpec
	Cameras      CameraTaps // nil: state-only episodes
	MinFreeBytes uint64     // 0: defaultMinFree
	Tick         time.Duration
	OnAutoStop   func(reason string)
}

type videoFrame struct {
	camID string
	au    []byte
	at    time.Time
}

type activeEpisode struct {
	id            string
	label         string
	start         time.Time
	w             *episodeWriter
	subs          []*nats.Subscription
	removeTap     func()
	vid           chan videoFrame
	drained       chan struct{}   // closed when drainVideo has flushed and exited
	videoChannels map[string]bool // cameras whose channel is registered
	dropped       atomic.Uint64
	cancel        chan struct{}
}

type Recorder struct {
	cfg Config
	mu  sync.Mutex
	ep  *activeEpisode
}

func New(cfg Config) *Recorder {
	if cfg.MinFreeBytes == 0 {
		cfg.MinFreeBytes = defaultMinFree
	}
	if cfg.Tick == 0 {
		cfg.Tick = time.Second
	}
	return &Recorder{cfg: cfg}
}

func newEpisodeID(now time.Time) string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return fmt.Sprintf("ep-%s-%s", now.UTC().Format("20060102T150405Z"), hex.EncodeToString(b))
}

func (r *Recorder) canStart() (bool, string) {
	if r.ep != nil {
		return false, "already recording " + r.ep.id
	}
	free, err := freeBytes(r.cfg.Dir)
	if err == nil && free < r.cfg.MinFreeBytes {
		return false, fmt.Sprintf("low disk: %d MiB free, %d MiB required",
			free>>20, r.cfg.MinFreeBytes>>20)
	}
	return true, ""
}

func (r *Recorder) StartEpisode(label string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ok, reason := r.canStart(); !ok {
		return "", fmt.Errorf("%s", reason)
	}
	now := time.Now()
	id := newEpisodeID(now)
	specs := r.cfg.Specs()
	w, err := newEpisodeWriter(r.cfg.Dir, id, specs)
	if err != nil {
		return "", err
	}
	ep := &activeEpisode{id: id, label: label, start: now, w: w, cancel: make(chan struct{})}

	for _, spec := range specs {
		topic := spec.Subject
		sub, err := r.cfg.NC.Subscribe("waypoint."+r.cfg.RoverID+"."+topic, func(m *nats.Msg) {
			_ = w.write(topic, m.Data, time.Now())
		})
		if err != nil {
			r.abort(ep)
			return "", err
		}
		ep.subs = append(ep.subs, sub)
	}

	if r.cfg.Cameras != nil {
		// The tap runs on the camera fanout goroutine: it must never block the
		// WebRTC path. Copy, try a buffered handoff, count the drop otherwise.
		ep.vid = make(chan videoFrame, 32)
		ep.drained = make(chan struct{})
		ep.videoChannels = map[string]bool{}
		var tapMu sync.Mutex
		started := map[string]bool{}
		ep.removeTap = r.cfg.Cameras.Tap(func(camID string, au []byte, keyframe bool) {
			tapMu.Lock()
			if !started[camID] {
				if !keyframe {
					tapMu.Unlock()
					return // keyframe-gated: drop until the stream is decodable
				}
				started[camID] = true
			}
			tapMu.Unlock()
			cp := make([]byte, len(au))
			copy(cp, au)
			select {
			case ep.vid <- videoFrame{camID: camID, au: cp, at: time.Now()}:
			default:
				ep.dropped.Add(1)
			}
		})
		go r.drainVideo(ep)
	}

	r.ep = ep
	go r.watch(ep)
	r.publishEventLocked("")
	return id, nil
}

// drainVideo serializes video writes off the fanout goroutine. It runs until
// ep.vid is closed (after the tap is removed in stopLocked), flushing every
// buffered frame so a normal stop loses nothing. Channels are registered lazily
// on a camera's first delivered keyframe.
func (r *Recorder) drainVideo(ep *activeEpisode) {
	defer close(ep.drained)
	for f := range ep.vid {
		if !ep.videoChannels[f.camID] {
			if err := ep.w.addVideoChannel(f.camID); err != nil {
				ep.dropped.Add(1)
				continue
			}
			ep.videoChannels[f.camID] = true
		}
		if err := ep.w.writeVideo(f.camID, f.au, f.at); err != nil {
			ep.dropped.Add(1)
		}
	}
}

// watch publishes progress and enforces the free-space guard while recording.
func (r *Recorder) watch(ep *activeEpisode) {
	t := time.NewTicker(r.cfg.Tick)
	defer t.Stop()
	for {
		select {
		case <-ep.cancel:
			return
		case <-t.C:
			free, err := freeBytes(r.cfg.Dir)
			if err == nil && free < r.cfg.MinFreeBytes {
				reason := fmt.Sprintf("low disk: %d MiB free", free>>20)
				r.autoStop(ep, reason)
				return
			}
			r.publishEvent("")
		}
	}
}

func (r *Recorder) autoStop(ep *activeEpisode, reason string) {
	r.mu.Lock()
	if r.ep != ep {
		r.mu.Unlock()
		return
	}
	_, _ = r.stopLocked(nil, "auto-stopped: "+reason)
	r.mu.Unlock()
	if r.cfg.OnAutoStop != nil {
		r.cfg.OnAutoStop(reason)
	}
}

func (r *Recorder) StopEpisode(success bool, notes string) (*Sidecar, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopLocked(&success, notes)
}

// stopLocked finalizes the active episode. success == nil means the outcome
// is unknown (auto-stop), which the sidecar records as null.
func (r *Recorder) stopLocked(success *bool, notes string) (*Sidecar, error) {
	ep := r.ep
	if ep == nil {
		return nil, fmt.Errorf("not recording")
	}
	close(ep.cancel)
	for _, s := range ep.subs {
		_ = s.Unsubscribe()
	}
	if ep.removeTap != nil {
		// removeTap returns only once the fanout goroutine can no longer send,
		// so closing ep.vid afterward is safe. Wait for drainVideo to flush the
		// buffered frames before finalizing, otherwise a write would race the
		// writer Close.
		ep.removeTap()
		close(ep.vid)
		<-ep.drained
	}
	end := time.Now()
	sc := &Sidecar{
		FormatVersion:      FormatVersion,
		EpisodeID:          ep.id,
		PlatformID:         r.cfg.PlatformID,
		RoverID:            r.cfg.RoverID,
		TaskLabel:          ep.label,
		Start:              ep.start.UTC(),
		End:                end.UTC(),
		DurationS:          end.Sub(ep.start).Seconds(),
		Success:            success,
		Notes:              notes,
		VideoFramesDropped: ep.dropped.Load(),
	}
	err := ep.w.finalize(sc)
	r.ep = nil
	r.publishEventLocked("")
	return sc, err
}

func (r *Recorder) abort(ep *activeEpisode) {
	for _, s := range ep.subs {
		_ = s.Unsubscribe()
	}
	_ = ep.w.w.Close()
	_ = ep.w.cf.f.Close()
	_ = os.Remove(partialPath(r.cfg.Dir, ep.id))
}

// List returns all episode sidecars, newest first.
func (r *Recorder) List() ([]*Sidecar, error) { return listSidecars(r.cfg.Dir) }

// Path resolves an episode id to its .mcap, refusing traversal and the
// in-flight episode.
func (r *Recorder) Path(id string) (string, error) {
	if id == "" || id != filepath.Base(id) || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid episode id")
	}
	r.mu.Lock()
	active := r.ep != nil && r.ep.id == id
	r.mu.Unlock()
	if active {
		return "", fmt.Errorf("episode is recording")
	}
	p := finalPath(r.cfg.Dir, id)
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

func (r *Recorder) Delete(id string) error {
	p, err := r.Path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	return os.Remove(filepath.Join(r.cfg.Dir, id+".json"))
}
