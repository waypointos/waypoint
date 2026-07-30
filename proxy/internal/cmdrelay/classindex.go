package cmdrelay

import (
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
)

// parseComponentCmd splits waypoint.<rover>.module.<id>.<token>.cmd. The token
// is returned unvalidated: only classIndex knows which ones are the module's
// declared component classes. Any other shape yields ok=false.
func parseComponentCmd(subject string) (roverID, moduleID, token string, ok bool) {
	p := strings.Split(subject, ".")
	if len(p) != 6 || p[0] != "waypoint" || p[2] != "module" || p[5] != "cmd" {
		return "", "", "", false
	}
	if p[1] == "" || p[3] == "" || p[4] == "" {
		return "", "", "", false
	}
	return p[1], p[3], p[4], true
}

// classIndex maps rover → module → declared component classes, learned from the
// rovers' infra.modules snapshots. The relay consults it so only a declared
// component leaf is forwardable: module.<id>.<class>.cmd. Everything else under
// module.<id> stays unreachable from a browser, in particular the agent's
// module.<id>.servo.cmd broker, which would otherwise let a session drive
// servos directly and bypass the module's interlocks.
type classIndex struct {
	mu      sync.RWMutex
	byRover map[string]map[string]map[string]bool
}

func newClassIndex() *classIndex {
	return &classIndex{byRover: map[string]map[string]map[string]bool{}}
}

// learn replaces a rover's entry from one ModuleSnapshot. Replacing rather than
// merging is what makes a detached module, or a changed class, drop its grant.
// An undecodable frame is ignored so a malformed snapshot cannot clear state.
func (c *classIndex) learn(roverID string, snapshot []byte) {
	var snap waypointv1.ModuleSnapshot
	if err := proto.Unmarshal(snapshot, &snap); err != nil {
		return
	}
	byModule := map[string]map[string]bool{}
	for _, m := range snap.GetModules() {
		classes := map[string]bool{}
		for _, comp := range m.GetComponents() {
			if cls := comp.GetClass(); cls != "" {
				classes[cls] = true
			}
		}
		// Agents that predate `components` only fill the singular field.
		if cls := m.GetComponent().GetClass(); cls != "" {
			classes[cls] = true
		}
		if len(classes) > 0 {
			byModule[m.GetId()] = classes
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byRover[roverID] = byModule
}

// allow reports whether subject is a declared component command leaf of a
// module currently running on that rover.
func (c *classIndex) allow(subject string) bool {
	roverID, moduleID, token, ok := parseComponentCmd(subject)
	if !ok {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byRover[roverID][moduleID][token]
}
