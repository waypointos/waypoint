package modules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// WriteDesiredCache persists the protobuf bytes for the most recently
// received DesiredModuleSet so the rover can boot offline with the last
// known set. Writes atomically via tmp + rename.
func WriteDesiredCache(path string, set *waypointv1.DesiredModuleSet) error {
	bytes, err := proto.Marshal(set)
	if err != nil {
		return fmt.Errorf("desired: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("desired: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("desired: write tmp: %w", err)
	}
	return os.Rename(tmp, path)
}

// ReadDesiredCache returns the cached set if present, or an empty set
// (no error) if the file does not exist.
func ReadDesiredCache(path string) (*waypointv1.DesiredModuleSet, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &waypointv1.DesiredModuleSet{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("desired: read: %w", err)
	}
	var set waypointv1.DesiredModuleSet
	if err := proto.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("desired: unmarshal: %w", err)
	}
	return &set, nil
}
