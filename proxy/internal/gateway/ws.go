package gateway

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/waypointos/waypoint/proxy/internal/authkit"
	"github.com/waypointos/waypoint/proxy/internal/db"
	"github.com/waypointos/waypoint/proxy/internal/operator"
)

type envelope struct {
	Action  string `json:"action"`  // sub, unsub, pub, msg, err
	Subject string `json:"subject"`
	Reply   string `json:"reply,omitempty"`
	Data    string `json:"data,omitempty"` // base64
}

type Deps struct {
	Access   *db.AccessRepo
	Rovers   *db.RoversRepo
	Operator *operator.Operator
	NATSURL  string
}

type Gateway struct {
	deps     Deps
	upgrader websocket.Upgrader
}

func New(d Deps) *Gateway {
	return &Gateway{
		deps:     d,
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
	}
}

func (g *Gateway) Handle(w http.ResponseWriter, r *http.Request) {
	u, ok := authkit.UserFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Build the session-access list. Admins implicitly receive admin role on
	// every rover in the fleet (same model as POST /api/auth/nats-cred).
	var sa []operator.SessionAccess
	if u.IsAdmin {
		rs, err := g.deps.Rovers.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, rv := range rs {
			sa = append(sa, operator.SessionAccess{RoverID: rv.ID, Role: "admin"})
		}
	} else {
		accesses, err := g.deps.Access.ListForUser(r.Context(), u.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, a := range accesses {
			sa = append(sa, operator.SessionAccess{RoverID: a.RoverID, Role: a.Role})
		}
	}

	jwtTok, seed, err := g.deps.Operator.MintSessionUserJWT(u.ID.String(), sa, operator.SessionJWTTTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	conn, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// writeMu serialises every websocket write. nats.go's ErrorHandler fires
	// from the connection's read loop goroutine, while subscription callbacks
	// fire from a separate dispatcher; without a mutex both can race with each
	// other (and with the main reader loop's response writes).
	var writeMu sync.Mutex
	writeJSON := func(env envelope) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.WriteJSON(env)
	}

	nc, err := nats.Connect(g.deps.NATSURL,
		nats.UserJWTAndSeed(jwtTok, string(seed)),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			if err == nil {
				return
			}
			writeJSON(envelope{Action: "err", Data: b64(err.Error())})
		}),
	)
	if err != nil {
		log.Printf("gateway nats connect: %v", err)
		return
	}
	defer nc.Close()

	// TODO(audit): wire audit.Publisher into gateway session events.

	subs := &subRegistry{m: map[string]*nats.Subscription{}}
	defer subs.unsubAll()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}
		switch env.Action {
		case "sub":
			if subs.has(env.Subject) {
				continue
			}
			sub, err := nc.Subscribe(env.Subject, func(msg *nats.Msg) {
				writeJSON(envelope{
					Action:  "msg",
					Subject: msg.Subject,
					Data:    base64.StdEncoding.EncodeToString(msg.Data),
				})
			})
			if err == nil {
				subs.add(env.Subject, sub)
			}
		case "unsub":
			subs.remove(env.Subject)
		case "pub":
			data, _ := base64.StdEncoding.DecodeString(env.Data)
			if env.Reply != "" {
				_ = nc.PublishRequest(env.Subject, env.Reply, data)
			} else {
				_ = nc.Publish(env.Subject, data)
			}
		}
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

type subRegistry struct {
	mu sync.Mutex
	m  map[string]*nats.Subscription
}

func (s *subRegistry) has(subj string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[subj]
	return ok
}
func (s *subRegistry) add(subj string, sub *nats.Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[subj] = sub
}
func (s *subRegistry) remove(subj string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sub, ok := s.m[subj]; ok {
		_ = sub.Unsubscribe()
		delete(s.m, subj)
	}
}
func (s *subRegistry) unsubAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.m {
		_ = sub.Unsubscribe()
	}
	s.m = nil
}
