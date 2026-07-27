package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const FormatVersion = 1

// Sidecar is the per-episode JSON metadata next to the .mcap. Its layout is a
// stability contract: exporter modules parse it (docs/setup/episodes.md).
type Sidecar struct {
	FormatVersion      int             `json:"format_version"`
	EpisodeID          string          `json:"episode_id"`
	PlatformID         string          `json:"platform_id"`
	RoverID            string          `json:"rover_id"`
	TaskLabel          string          `json:"task_label"`
	Start              time.Time       `json:"start"`
	End                time.Time       `json:"end"`
	DurationS          float64         `json:"duration_s"`
	Success            *bool           `json:"success"`
	Notes              string          `json:"notes"`
	Crashed            bool            `json:"crashed"`
	Bytes              uint64          `json:"bytes"`
	VideoFramesDropped uint64          `json:"video_frames_dropped"`
	Streams            []SidecarStream `json:"streams"`
}

type SidecarStream struct {
	Subject string `json:"subject"`
	Message string `json:"message"`
	Count   uint64 `json:"count"`
}

func (s *Sidecar) write(dir string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.EpisodeID+".json"), b, 0o644)
}

func loadSidecar(path string) (*Sidecar, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Sidecar
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// listSidecars returns all episode metadata in dir, newest first.
func listSidecars(dir string) ([]*Sidecar, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Sidecar
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		s, err := loadSidecar(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // a malformed sidecar must not break listing
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EpisodeID > out[j].EpisodeID })
	return out, nil
}

func (s *Sidecar) toProto() *waypointv1.EpisodeMeta {
	m := &waypointv1.EpisodeMeta{
		EpisodeId:          s.EpisodeID,
		PlatformId:         s.PlatformID,
		RoverId:            s.RoverID,
		TaskLabel:          s.TaskLabel,
		Start:              timestamppb.New(s.Start),
		End:                timestamppb.New(s.End),
		DurationS:          s.DurationS,
		Notes:              s.Notes,
		Crashed:            s.Crashed,
		Bytes:              s.Bytes,
		FormatVersion:      uint32(s.FormatVersion),
		VideoFramesDropped: s.VideoFramesDropped,
	}
	if s.Success != nil {
		m.Success = proto.Bool(*s.Success)
	}
	for _, st := range s.Streams {
		m.Streams = append(m.Streams, &waypointv1.EpisodeStream{
			Subject: st.Subject, Message: st.Message, Count: st.Count,
		})
	}
	return m
}
