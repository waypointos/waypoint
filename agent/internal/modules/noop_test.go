//go:build !linux

package modules

import (
	"context"
	"testing"
)

func TestNoopSupervisor_Lifecycle(t *testing.T) {
	sup := NewNoopSupervisor()
	ctx := context.Background()
	if err := sup.Start(ctx, "umr"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st := sup.Status("umr"); st.State != StateRunning {
		t.Fatalf("Status after Start: %+v", st)
	}
	if err := sup.Stop(ctx, "umr"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if st := sup.Status("umr"); st.State != StateStopped {
		t.Fatalf("Status after Stop: %+v", st)
	}
}
