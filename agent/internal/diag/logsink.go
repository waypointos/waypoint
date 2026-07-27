// BusSink is an slog.Handler that marshals each record into a waypointv1.LogRecord
// and publishes it on the local NATS bus at the rover's diag.log subject.
//
// Failures are dropped, never propagated. A logger sink that blocks or panics
// would take the whole agent down with it.
package diag

import (
	"context"
	"log/slog"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// Publisher is the minimum surface BusSink needs from a NATS connection.
// Both *nats.Conn and the in-process embedded server expose this method.
type Publisher interface {
	Publish(subject string, data []byte) error
}

// BusSink fans slog records onto the rover's NATS bus.
//
// dropped is held by pointer so handlers derived via WithAttrs / WithGroup
// share the counter with the root sink — child sinks would otherwise need to
// be copied along with their atomic, which go vet correctly flags.
type BusSink struct {
	bus     Publisher
	subject string
	level   slog.Level

	groups []string
	attrs  []slog.Attr

	dropped *atomic.Uint64
}

// NewBusSink returns a BusSink that publishes to the given subject at or above
// the given level.
func NewBusSink(bus Publisher, subject string, level slog.Level) *BusSink {
	return &BusSink{bus: bus, subject: subject, level: level, dropped: new(atomic.Uint64)}
}

// Dropped returns the count of records that failed to publish.
func (s *BusSink) Dropped() uint64 { return s.dropped.Load() }

func (s *BusSink) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= s.level
}

func (s *BusSink) Handle(_ context.Context, r slog.Record) error {
	fields := map[string]string{}
	for _, a := range s.attrs {
		fields[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.String()
		return true
	})

	source := ""
	if len(s.groups) > 0 {
		source = s.groups[len(s.groups)-1]
	}

	rec := &waypointv1.LogRecord{
		TsNs:   r.Time.UnixNano(),
		Level:  levelString(r.Level),
		Msg:    r.Message,
		Source: &source,
		Fields: fields,
	}

	data, err := proto.Marshal(rec)
	if err != nil {
		s.dropped.Add(1)
		return nil
	}

	if err := s.bus.Publish(s.subject, data); err != nil {
		s.dropped.Add(1)
	}
	return nil
}

func (s *BusSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BusSink{
		bus:     s.bus,
		subject: s.subject,
		level:   s.level,
		groups:  s.groups,
		attrs:   append(append([]slog.Attr{}, s.attrs...), attrs...),
		dropped: s.dropped,
	}
}

func (s *BusSink) WithGroup(name string) slog.Handler {
	return &BusSink{
		bus:     s.bus,
		subject: s.subject,
		level:   s.level,
		groups:  append(append([]string{}, s.groups...), name),
		attrs:   s.attrs,
		dropped: s.dropped,
	}
}

func levelString(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}

// Compile-time check that BusSink satisfies slog.Handler.
var _ slog.Handler = (*BusSink)(nil)
