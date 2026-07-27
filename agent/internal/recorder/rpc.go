package recorder

import (
	"time"

	"github.com/nats-io/nats.go"
	waypointv1 "github.com/waypointos/waypoint/protocol/gen/go/messages"
	"github.com/waypointos/waypoint/protocol/platform/stamp"
	"google.golang.org/protobuf/proto"
)

const idleEventEvery = 15 * time.Second

// Serve subscribes the recorder RPCs and starts the idle status heartbeat.
func (r *Recorder) Serve() error {
	pre := "waypoint." + r.cfg.RoverID + "."
	if _, err := r.cfg.NC.Subscribe(pre+"rpc.recorder_start", r.onStart); err != nil {
		return err
	}
	if _, err := r.cfg.NC.Subscribe(pre+"rpc.recorder_stop", r.onStop); err != nil {
		return err
	}
	if _, err := r.cfg.NC.Subscribe(pre+"rpc.episode_list", r.onList); err != nil {
		return err
	}
	go r.idleHeartbeat()
	r.publishEvent("")
	return nil
}

func (r *Recorder) idleHeartbeat() {
	t := time.NewTicker(idleEventEvery)
	defer t.Stop()
	for range t.C {
		r.mu.Lock()
		idle := r.ep == nil
		r.mu.Unlock()
		if idle {
			r.publishEvent("")
		}
	}
}

func (r *Recorder) onStart(msg *nats.Msg) {
	var req waypointv1.RecorderStartRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		r.respond(msg, &waypointv1.RecorderStartResponse{Reason: "bad request"})
		return
	}
	id, err := r.StartEpisode(req.TaskLabel)
	if err != nil {
		// The refusal also rides event.recorder so sessions that did not
		// issue the request see why the control is unavailable.
		r.publishEvent(err.Error())
		r.respond(msg, &waypointv1.RecorderStartResponse{Reason: err.Error()})
		return
	}
	r.respond(msg, &waypointv1.RecorderStartResponse{Ok: true, EpisodeId: id})
}

func (r *Recorder) onStop(msg *nats.Msg) {
	var req waypointv1.RecorderStopRequest
	if err := proto.Unmarshal(msg.Data, &req); err != nil {
		r.respond(msg, &waypointv1.RecorderStopResponse{Reason: "bad request"})
		return
	}
	sc, err := r.StopEpisode(req.Success, req.Notes)
	if err != nil {
		r.respond(msg, &waypointv1.RecorderStopResponse{Reason: err.Error()})
		return
	}
	r.respond(msg, &waypointv1.RecorderStopResponse{Ok: true, Summary: sc.toProto()})
}

func (r *Recorder) onList(msg *nats.Msg) {
	scs, err := r.List()
	if err != nil {
		r.respond(msg, &waypointv1.EpisodeList{})
		return
	}
	out := &waypointv1.EpisodeList{}
	for _, sc := range scs {
		out.Episodes = append(out.Episodes, sc.toProto())
	}
	r.respond(msg, out)
}

func (r *Recorder) respond(msg *nats.Msg, m proto.Message) {
	if msg.Reply == "" {
		return
	}
	body, err := proto.Marshal(m)
	if err != nil {
		return
	}
	_ = r.cfg.NC.Publish(msg.Reply, body)
}

// publishEvent locks and delegates; use from unlocked contexts only
// (watch, idleHeartbeat, the onStart refusal path).
func (r *Recorder) publishEvent(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publishEventLocked(reason)
}

// publishEventLocked builds and publishes the status event. Callers hold
// r.mu. NATS Publish does not call back into the recorder, so publishing
// under the lock is safe.
func (r *Recorder) publishEventLocked(reason string) {
	ev := &waypointv1.RecorderEvent{
		Stamp:  stamp.Now(),
		State:  waypointv1.RecorderState_RECORDER_STATE_IDLE,
		Reason: reason,
	}
	if r.ep != nil {
		ev.State = waypointv1.RecorderState_RECORDER_STATE_RECORDING
		ev.EpisodeId = r.ep.id
		ev.ElapsedS = time.Since(r.ep.start).Seconds()
		ev.BytesWritten = r.ep.w.bytesWritten()
	} else if ok, why := r.canStart(); ok {
		ev.CanStart = true
	} else if reason == "" {
		ev.Reason = why
	}
	body, err := proto.Marshal(ev)
	if err != nil {
		return
	}
	_ = r.cfg.NC.Publish("waypoint."+r.cfg.RoverID+".event.recorder", body)
}
