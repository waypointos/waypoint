package diag

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestMultiHandlerFansOut(t *testing.T) {
	var a, b bytes.Buffer
	h := NewMultiHandler(
		slog.NewTextHandler(&a, nil),
		slog.NewTextHandler(&b, nil),
	)
	logger := slog.New(h)
	logger.Info("hello", "k", "v")

	if !strings.Contains(a.String(), "hello") {
		t.Fatalf("handler A missed record: %q", a.String())
	}
	if !strings.Contains(b.String(), "hello") {
		t.Fatalf("handler B missed record: %q", b.String())
	}
}

func TestMultiHandlerEnabledIsOr(t *testing.T) {
	h := NewMultiHandler(
		// debug-only
		slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug}),
		// error-only
		slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}),
	)
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("Enabled should be true when any child is enabled")
	}
}

type errHandler struct{}

func (h *errHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (h *errHandler) Handle(context.Context, slog.Record) error { return errors.New("boom") }
func (h *errHandler) WithAttrs(_ []slog.Attr) slog.Handler      { return h }
func (h *errHandler) WithGroup(_ string) slog.Handler           { return h }

func TestMultiHandlerOneErrorDoesNotSuppressOthers(t *testing.T) {
	var ok bytes.Buffer
	h := NewMultiHandler(&errHandler{}, slog.NewTextHandler(&ok, nil))
	slog.New(h).Info("survives")
	if !strings.Contains(ok.String(), "survives") {
		t.Fatalf("second handler missed record after first errored: %q", ok.String())
	}
}
