package recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/foxglove/mcap/go/mcap"
	foxglovepb "github.com/waypointos/waypoint/protocol/gen/go/foxglove"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const videoSchemaName = "foxglove.CompressedVideo"

type channelState struct {
	id      uint16
	message string
	count   uint64
}

// countingFile tracks bytes written so progress events can report size
// without stat-ing the file.
type countingFile struct {
	f *os.File
	n uint64
}

func (c *countingFile) Write(p []byte) (int, error) {
	n, err := c.f.Write(p)
	c.n += uint64(n)
	return n, err
}

// episodeWriter owns one .mcap.partial file. All methods are safe for
// concurrent use; the mcap writer itself is not, hence the mutex.
type episodeWriter struct {
	mu         sync.Mutex
	dir        string
	id         string
	cf         *countingFile
	w          *mcap.Writer
	channels   map[string]*channelState // by topic
	schemas    map[string]uint16        // schema id by message full name
	nextChan   uint16
	nextSchema uint16
	seq        uint32
}

func partialPath(dir, id string) string { return filepath.Join(dir, id+".mcap.partial") }
func finalPath(dir, id string) string   { return filepath.Join(dir, id+".mcap") }

func newEpisodeWriter(dir, id string, specs []StreamSpec) (*episodeWriter, error) {
	f, err := os.OpenFile(partialPath(dir, id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	cf := &countingFile{f: f}
	w, err := mcap.NewWriter(cf, &mcap.WriterOptions{
		Chunked:     true,
		ChunkSize:   1 << 20,
		Compression: mcap.CompressionZSTD,
	})
	if err != nil {
		f.Close()
		return nil, err
	}
	if err := w.WriteHeader(&mcap.Header{Profile: ""}); err != nil {
		f.Close()
		return nil, err
	}
	ew := &episodeWriter{
		dir: dir, id: id, cf: cf, w: w,
		channels: map[string]*channelState{},
		schemas:  map[string]uint16{},
	}
	for _, s := range specs {
		if _, err := ew.addChannel(s.Subject, s.Message); err != nil {
			f.Close()
			return nil, err
		}
	}
	return ew, nil
}

// addChannel registers a schema (once per message name) and a channel for the
// topic. An unknown message name yields a schemaless channel (id 0) rather
// than an error: the bytes are still worth keeping.
func (ew *episodeWriter) addChannel(topic, message string) (*channelState, error) {
	if ch, ok := ew.channels[topic]; ok {
		return ch, nil
	}
	schemaID := uint16(0)
	if message != "" {
		if id, ok := ew.schemas[message]; ok {
			schemaID = id
		} else if data, err := fileDescriptorSetFor(message); err == nil {
			ew.nextSchema++
			schemaID = ew.nextSchema
			if err := ew.w.WriteSchema(&mcap.Schema{
				ID: schemaID, Name: message, Encoding: "protobuf", Data: data,
			}); err != nil {
				return nil, err
			}
			ew.schemas[message] = schemaID
		}
	}
	id := ew.nextChan
	ew.nextChan++
	if err := ew.w.WriteChannel(&mcap.Channel{
		ID: id, SchemaID: schemaID, Topic: topic, MessageEncoding: "protobuf",
	}); err != nil {
		return nil, err
	}
	ch := &channelState{id: id, message: message}
	ew.channels[topic] = ch
	return ch, nil
}

func (ew *episodeWriter) addVideoChannel(camID string) error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	_, err := ew.addChannel("camera."+camID+"/h264", videoSchemaName)
	return err
}

func (ew *episodeWriter) write(topic string, data []byte, at time.Time) error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	ch, ok := ew.channels[topic]
	if !ok {
		return fmt.Errorf("no channel for topic %s", topic)
	}
	ns := uint64(at.UnixNano())
	ew.seq++
	if err := ew.w.WriteMessage(&mcap.Message{
		ChannelID: ch.id, Sequence: ew.seq, LogTime: ns, PublishTime: ns, Data: data,
	}); err != nil {
		return err
	}
	ch.count++
	return nil
}

func (ew *episodeWriter) writeVideo(camID string, au []byte, at time.Time) error {
	frame := &foxglovepb.CompressedVideo{
		Timestamp: timestamppb.New(at),
		FrameId:   camID,
		Data:      au,
		Format:    "h264",
	}
	b, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	return ew.write("camera."+camID+"/h264", b, at)
}

func (ew *episodeWriter) bytesWritten() uint64 {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	return ew.cf.n
}

// finalize fills the sidecar's stream/byte counts, embeds a copy of the
// metadata in the container, closes it, renames .partial away, and writes
// the sidecar.
func (ew *episodeWriter) finalize(sc *Sidecar) error {
	ew.mu.Lock()
	for topic, ch := range ew.channels {
		sc.Streams = append(sc.Streams, SidecarStream{Subject: topic, Message: ch.message, Count: ch.count})
	}
	sort.Slice(sc.Streams, func(i, j int) bool { return sc.Streams[i].Subject < sc.Streams[j].Subject })
	// The final size is unknowable before Close, so the embedded copy carries
	// a best-effort byte count; the JSON sidecar below gets the exact one.
	sc.Bytes = ew.cf.n
	meta, err := json.Marshal(sc)
	if err == nil {
		_ = ew.w.WriteMetadata(&mcap.Metadata{
			Name: "waypoint.episode", Metadata: map[string]string{"sidecar": string(meta)},
		})
	}
	err = ew.w.Close()
	ew.cf.f.Close()
	sc.Bytes = ew.cf.n
	ew.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.Rename(partialPath(ew.dir, ew.id), finalPath(ew.dir, ew.id)); err != nil {
		return err
	}
	return sc.write(ew.dir)
}
