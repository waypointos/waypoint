package modules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/waypointos/waypoint/modverify"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

var ErrSANMismatch = errors.New("signer SAN does not match the pinned identity for this module")

// ReadLocalDesired / WriteLocalDesired name the locally-installed module set
// (modules.local.pb) distinctly from the proxy cache, though both use the same
// DesiredModuleSet wire format.
func ReadLocalDesired(path string) (*waypointv1.DesiredModuleSet, error) {
	return ReadDesiredCache(path)
}

func WriteLocalDesired(path string, set *waypointv1.DesiredModuleSet) error {
	return WriteDesiredCache(path, set)
}

// mergeDesired returns the union of proxy and local sets, keyed by Id.
// Local entries win on conflict; timestamp is set to now. The result aliases
// the source *ModuleDesired pointers, so callers must not mutate the entries.
func mergeDesired(proxy, local *waypointv1.DesiredModuleSet) *waypointv1.DesiredModuleSet {
	out := &waypointv1.DesiredModuleSet{T: timestamppb.New(time.Now().UTC())}
	seen := map[string]bool{}
	if local != nil {
		for _, m := range local.Modules {
			out.Modules = append(out.Modules, m)
			seen[m.Id] = true
		}
	}
	if proxy != nil {
		for _, m := range proxy.Modules {
			if !seen[m.Id] {
				out.Modules = append(out.Modules, m)
			}
		}
	}
	return out
}

// upsertModule replaces the entry with the same Id (in place) or appends it.
// Mutates set; call only on a set the caller owns (e.g. the manager's local store),
// not on the ephemeral result of mergeDesired.
func upsertModule(set *waypointv1.DesiredModuleSet, m *waypointv1.ModuleDesired) *waypointv1.DesiredModuleSet {
	if set == nil {
		set = &waypointv1.DesiredModuleSet{}
	}
	for i, e := range set.Modules {
		if e.Id == m.Id {
			set.Modules[i] = m
			return set
		}
	}
	set.Modules = append(set.Modules, m)
	return set
}

// removeModule returns a set without the given id. Nil-safe.
func removeModule(set *waypointv1.DesiredModuleSet, id string) *waypointv1.DesiredModuleSet {
	if set == nil {
		return &waypointv1.DesiredModuleSet{}
	}
	out := &waypointv1.DesiredModuleSet{T: set.T}
	for _, m := range set.Modules {
		if m.Id != id {
			out.Modules = append(out.Modules, m)
		}
	}
	return out
}

func containsModule(set *waypointv1.DesiredModuleSet, id string) bool {
	if set == nil {
		return false
	}
	for _, m := range set.Modules {
		if m.Id == id {
			return true
		}
	}
	return false
}

type trustFile struct {
	Pins map[string]string `toml:"pins"`
}

func readTrust(path string) (trustFile, error) {
	tf := trustFile{Pins: map[string]string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return tf, nil
	}
	if err != nil {
		return tf, err
	}
	if _, err := toml.Decode(string(data), &tf); err != nil {
		return tf, fmt.Errorf("trust: decode: %w", err)
	}
	if tf.Pins == nil {
		tf.Pins = map[string]string{}
	}
	return tf, nil
}

// CheckPinnedSAN returns nil if id has no pin yet (TOFU) or the pinned signer
// matches san. Comparison is on the workflow identity (SAN minus the git ref),
// so a new release from the same workflow passes while a different repo or
// workflow trips ErrSANMismatch. Normalizing both sides also migrates pins
// stored as full SANs before this became identity-based.
func CheckPinnedSAN(path, id, san string) error {
	tf, err := readTrust(path)
	if err != nil {
		return err
	}
	if pinned, ok := tf.Pins[id]; ok && modverify.SigningIdentity(pinned) != modverify.SigningIdentity(san) {
		return ErrSANMismatch
	}
	return nil
}

// PinSAN records id→signer-identity in the trust store (idempotent overwrite).
func PinSAN(path, id, san string) error {
	tf, err := readTrust(path)
	if err != nil {
		return err
	}
	tf.Pins[id] = modverify.SigningIdentity(san)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeTOML(path, tf)
}
