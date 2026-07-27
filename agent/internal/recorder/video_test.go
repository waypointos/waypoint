package recorder

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/foxglove/mcap/go/mcap"
	"github.com/stretchr/testify/require"
)

// fakeCameraTaps is a controllable CameraTaps for exercising the video path.
type fakeCameraTaps struct {
	mu sync.Mutex
	fn func(camID string, au []byte, keyframe bool)
}

func (f *fakeCameraTaps) Tap(fn func(camID string, au []byte, keyframe bool)) (remove func()) {
	f.mu.Lock()
	f.fn = fn
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		f.fn = nil
		f.mu.Unlock()
	}
}

func (f *fakeCameraTaps) emit(camID string, au []byte, keyframe bool) {
	f.mu.Lock()
	fn := f.fn
	f.mu.Unlock()
	if fn != nil {
		fn(camID, au, keyframe)
	}
}

// TestVideoFramesDrainedBeforeFinalize fills the video buffer and stops without
// pacing the drain goroutine. Every buffered frame must reach the file: none
// dropped, and no write may land after the writer is closed.
func TestVideoFramesDrainedBeforeFinalize(t *testing.T) {
	nc := startEmbeddedNATS(t)
	dir := t.TempDir()
	taps := &fakeCameraTaps{}
	r := New(Config{
		NC:      nc,
		RoverID: "dev",
		Dir:     dir,
		Specs:   func() []StreamSpec { return nil },
		Cameras: taps,
		Tick:    time.Hour, // keep the watch goroutine out of the way
	})
	require.NoError(t, r.Serve())

	id, err := r.StartEpisode("vid")
	require.NoError(t, err)

	// First frame must be a keyframe to open the stream, then fill the buffer.
	const n = 32
	au := make([]byte, 1024)
	for i := 0; i < n; i++ {
		taps.emit("cam0", au, i == 0)
	}

	sc, err := r.StopEpisode(true, "")
	require.NoError(t, err)
	require.Equal(t, uint64(0), sc.VideoFramesDropped, "no buffered frame should be dropped")

	var videoCount uint64
	for _, st := range sc.Streams {
		if st.Subject == "camera.cam0/h264" {
			videoCount = st.Count
		}
	}
	require.Equal(t, uint64(n), videoCount, "all buffered frames must be written")

	// The finalized file opens cleanly and reports the same count.
	f, err := os.Open(filepath.Join(dir, id+".mcap"))
	require.NoError(t, err)
	defer f.Close()
	rdr, err := mcap.NewReader(f)
	require.NoError(t, err)
	info, err := rdr.Info()
	require.NoError(t, err)
	require.Equal(t, uint64(n), info.Statistics.MessageCount)
}
