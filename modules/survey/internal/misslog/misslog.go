// Package misslog writes the mission CSV: 5 Hz pose samples plus event
// rows, one file per process start.
package misslog

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var header = []string{"t", "x", "y", "heading_deg", "speed_mps", "state", "detected_id", "event"}

type Logger struct {
	f *os.File
	w *csv.Writer
}

// New creates log_dir if needed and opens a timestamped CSV.
func New(dir string) (*Logger, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", fmt.Errorf("misslog: %w", err)
	}
	path := filepath.Join(dir, "mission-"+time.Now().UTC().Format("20060102-150405")+".csv")
	f, err := os.Create(path)
	if err != nil {
		return nil, "", fmt.Errorf("misslog: %w", err)
	}
	l := &Logger{f: f, w: csv.NewWriter(f)}
	if err := l.w.Write(header); err != nil {
		f.Close()
		return nil, "", err
	}
	l.w.Flush()
	return l, path, nil
}

// Row appends one line. detectedID < 0 renders empty; event is empty for
// periodic samples.
func (l *Logger) Row(t time.Time, x, y, headingDeg, speed float64, state string, detectedID int, event string) error {
	id := ""
	if detectedID >= 0 {
		id = strconv.Itoa(detectedID)
	}
	rec := []string{
		strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', 3, 64),
		strconv.FormatFloat(x, 'f', 3, 64),
		strconv.FormatFloat(y, 'f', 3, 64),
		strconv.FormatFloat(headingDeg, 'f', 1, 64),
		strconv.FormatFloat(speed, 'f', 3, 64),
		state,
		id,
		event,
	}
	if err := l.w.Write(rec); err != nil {
		return err
	}
	l.w.Flush()
	return l.w.Error()
}

func (l *Logger) Close() error {
	l.w.Flush()
	return l.f.Close()
}
