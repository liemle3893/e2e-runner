package adapter_test

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/liemle3893/go-tryve/internal/adapter"
)

func TestProcessManager_StartAndGet(t *testing.T) {
	pm := adapter.NewProcessManager()
	ctx := context.Background()

	proc, err := pm.Start(ctx, adapter.StartOpts{
		Name:            "test-proc",
		Command:         "sleep 60",
		AutoTeardown:    true,
		TeardownTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.StopAll()

	if proc.PID <= 0 {
		t.Fatalf("expected positive PID, got %d", proc.PID)
	}

	got, ok := pm.Get("test-proc")
	if !ok {
		t.Fatal("expected Get to find the process")
	}
	if got.PID != proc.PID {
		t.Fatalf("PID mismatch: got %d, want %d", got.PID, proc.PID)
	}
}

func TestProcessManager_DuplicateName(t *testing.T) {
	pm := adapter.NewProcessManager()
	ctx := context.Background()

	_, err := pm.Start(ctx, adapter.StartOpts{
		Name:         "dup",
		Command:      "sleep 60",
		AutoTeardown: true,
	})
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer pm.StopAll()

	_, err = pm.Start(ctx, adapter.StartOpts{
		Name:         "dup",
		Command:      "sleep 60",
		AutoTeardown: true,
	})
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestProcessManager_GracefulStop(t *testing.T) {
	pm := adapter.NewProcessManager()
	ctx := context.Background()

	proc, err := pm.Start(ctx, adapter.StartOpts{
		Name:         "graceful",
		Command:      "sleep 60",
		AutoTeardown: true,
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := pm.Stop("graceful", syscall.SIGTERM, 5*time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	select {
	case <-proc.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("process did not exit after Stop")
	}

	_, ok := pm.Get("graceful")
	if ok {
		t.Fatal("expected process to be removed from manager after Stop")
	}
}

func TestProcessManager_StopAll(t *testing.T) {
	pm := adapter.NewProcessManager()
	ctx := context.Background()

	procs := make([]*adapter.ManagedProcess, 3)
	for i := 0; i < 3; i++ {
		var err error
		procs[i], err = pm.Start(ctx, adapter.StartOpts{
			Name:            "proc-" + string(rune('a'+i)),
			Command:         "sleep 60",
			AutoTeardown:    true,
			TeardownTimeout: 2 * time.Second,
		})
		if err != nil {
			t.Fatalf("Start %d failed: %v", i, err)
		}
	}

	pm.StopAll()

	for i, proc := range procs {
		select {
		case <-proc.Done():
		case <-time.After(5 * time.Second):
			t.Fatalf("process %d did not exit after StopAll", i)
		}
	}
}

func TestProcessManager_StopNonexistent(t *testing.T) {
	pm := adapter.NewProcessManager()
	err := pm.Stop("no-such-proc", syscall.SIGTERM, 2*time.Second)
	if err == nil {
		t.Fatal("expected error for stopping non-existent process")
	}
}

func TestProcessManager_ProcessExitCapture(t *testing.T) {
	pm := adapter.NewProcessManager()
	ctx := context.Background()

	proc, err := pm.Start(ctx, adapter.StartOpts{
		Name:         "short",
		Command:      "echo hello && exit 0",
		AutoTeardown: true,
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.StopAll()

	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}

	stdout := proc.Stdout()
	if stdout == "" {
		t.Fatal("expected captured stdout")
	}
}
