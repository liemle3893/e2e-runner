package adapter_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/liemle3893/go-tryve/internal/adapter"
)

func TestProcessAdapter_StartStop(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()

	result, err := a.Execute(ctx, "start", map[string]any{
		"command": "sleep 60",
		"name":    "test-server",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	pid, ok := result.Data["pid"].(float64)
	if !ok || pid <= 0 {
		t.Fatalf("expected positive pid, got %v", result.Data["pid"])
	}

	port, ok := result.Data["port"].(float64)
	if !ok || port <= 0 {
		t.Fatalf("expected positive port, got %v", result.Data["port"])
	}

	result, err = a.Execute(ctx, "stop", map[string]any{
		"target":  "test-server",
		"timeout": "2s",
	})
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if result.Data["stopped"] != "test-server" {
		t.Fatalf("expected stopped=test-server, got %v", result.Data["stopped"])
	}
}

func TestProcessAdapter_StopByPID(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()

	result, err := a.Execute(ctx, "start", map[string]any{
		"command": "sleep 60",
		"name":    "pid-test",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	pid := result.Data["pid"].(float64)

	result, err = a.Execute(ctx, "stop", map[string]any{
		"pid":     pid,
		"timeout": "2s",
	})
	if err != nil {
		t.Fatalf("stop by PID failed: %v", err)
	}
}

func TestProcessAdapter_FreePortAllocation(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()
	defer a.Close(ctx)

	result, err := a.Execute(ctx, "start", map[string]any{
		"command": "sleep 60",
		"name":    "port-test",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	port := int(result.Data["port"].(float64))

	// Verify the port is in a valid range.
	if port < 1024 || port > 65535 {
		t.Fatalf("port %d is outside expected range", port)
	}
}

func TestProcessAdapter_FreePortInjection(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()
	defer a.Close(ctx)

	result, err := a.Execute(ctx, "start", map[string]any{
		"command": "echo listening on {{free_port}}",
		"name":    "inject-test",
		"env": map[string]any{
			"MY_PORT": "{{free_port}}",
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	port := result.Data["port"].(float64)
	if port <= 0 {
		t.Fatal("expected positive port")
	}
}

func TestProcessAdapter_ReadinessHTTP(t *testing.T) {
	// Start a real HTTP server to probe against.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srvPort := l.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})}
	go srv.Serve(l)
	defer srv.Close()

	a := adapter.NewProcessAdapter()
	ctx := context.Background()
	defer a.Close(ctx)

	result, err := a.Execute(ctx, "start", map[string]any{
		"command": "sleep 60",
		"name":    "readiness-http-test",
		"readiness": map[string]any{
			"http":     fmt.Sprintf("http://127.0.0.1:%d/", srvPort),
			"timeout":  "5s",
			"interval": "200ms",
		},
	})
	if err != nil {
		t.Fatalf("start with readiness failed: %v", err)
	}

	if result.Data["pid"].(float64) <= 0 {
		t.Fatal("expected positive pid after readiness check")
	}
}

func TestProcessAdapter_ReadinessTimeout(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()
	defer a.Close(ctx)

	_, err := a.Execute(ctx, "start", map[string]any{
		"command": "sleep 60",
		"name":    "readiness-timeout-test",
		"readiness": map[string]any{
			"http":     "http://127.0.0.1:1/no-server-here",
			"timeout":  "500ms",
			"interval": "100ms",
		},
	})
	if err == nil {
		t.Fatal("expected readiness timeout error")
	}
}

func TestProcessAdapter_CloseStopsAll(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := a.Execute(ctx, "start", map[string]any{
			"command":          "sleep 60",
			"name":             fmt.Sprintf("close-test-%d", i),
			"auto_teardown":    true,
			"teardown_timeout": "2s",
		})
		if err != nil {
			t.Fatalf("start %d failed: %v", i, err)
		}
	}

	if err := a.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestProcessAdapter_InvalidAction(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()

	_, err := a.Execute(ctx, "restart", map[string]any{})
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestProcessAdapter_StopNonexistent(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()

	_, err := a.Execute(ctx, "stop", map[string]any{
		"target": "does-not-exist",
	})
	if err == nil {
		t.Fatal("expected error for stopping non-existent process")
	}
}

func TestProcessAdapter_StopMissingTargetAndPID(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()

	_, err := a.Execute(ctx, "stop", map[string]any{})
	if err == nil {
		t.Fatal("expected error when neither target nor pid provided")
	}
}

func TestProcessAdapter_ReadinessCmd(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()
	defer a.Close(ctx)

	result, err := a.Execute(ctx, "start", map[string]any{
		"command": "sleep 60",
		"name":    "readiness-cmd-test",
		"readiness": map[string]any{
			"cmd":      "true",
			"timeout":  "5s",
			"interval": "200ms",
		},
	})
	if err != nil {
		t.Fatalf("start with cmd readiness failed: %v", err)
	}

	if result.Data["pid"].(float64) <= 0 {
		t.Fatal("expected positive pid after readiness check")
	}
}

func TestProcessAdapter_AutoTeardownFalse(t *testing.T) {
	a := adapter.NewProcessAdapter()
	ctx := context.Background()

	_, err := a.Execute(ctx, "start", map[string]any{
		"command":       "sleep 60",
		"name":          "no-auto-teardown",
		"auto_teardown": false,
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Close should stop ALL processes (including auto_teardown: false) to prevent leaks.
	if err := a.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
