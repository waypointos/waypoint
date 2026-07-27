// Fan-out slog.Handler. Stdlib slog ships no MultiHandler, so the agent
// builds one here. WithAttrs and WithGroup recurse so structured context
// propagates to every child.
package diag

import (
	"context"
	"log/slog"
)

// MultiHandler implements slog.Handler by delegating to a slice of children.
type MultiHandler struct {
	children []slog.Handler
}

// NewMultiHandler returns a handler that fans every record to all children.
func NewMultiHandler(children ...slog.Handler) *MultiHandler {
	return &MultiHandler{children: children}
}

func (h *MultiHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	for _, c := range h.children {
		if c.Enabled(ctx, lvl) {
			return true
		}
	}
	return false
}

// Handle forwards the record to every child. Errors from individual children
// are discarded — a failing sink must never suppress healthier ones.
func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, c := range h.children {
		_ = c.Handle(ctx, r)
	}
	return nil
}

func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(h.children))
	for i, c := range h.children {
		out[i] = c.WithAttrs(attrs)
	}
	return &MultiHandler{children: out}
}

func (h *MultiHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(h.children))
	for i, c := range h.children {
		out[i] = c.WithGroup(name)
	}
	return &MultiHandler{children: out}
}
