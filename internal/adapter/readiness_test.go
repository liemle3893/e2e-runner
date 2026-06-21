package adapter_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/liemle3893/go-tryve/internal/adapter"
)

func TestWaitForReady_HTTP_Success(t *testing.T) {
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(l)
	defer srv.Close()

	cfg := adapter.ReadinessConfig{
		HTTP:     fmt.Sprintf("http://127.0.0.1:%d/", l.Addr().(*net.TCPAddr).Port),
		Timeout:  5 * time.Second,
		Interval: 100 * time.Millisecond,
	}
	done := make(chan struct{})
	if err := adapter.WaitForReady(context.Background(), cfg, done); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestWaitForReady_HTTP_Timeout(t *testing.T) {
	cfg := adapter.ReadinessConfig{
		HTTP:     "http://127.0.0.1:1/no-server-here",
		Timeout:  500 * time.Millisecond,
		Interval: 100 * time.Millisecond,
	}
	done := make(chan struct{})
	err := adapter.WaitForReady(context.Background(), cfg, done)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForReady_TCP_Success(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	cfg := adapter.ReadinessConfig{
		TCP:      l.Addr().String(),
		Timeout:  5 * time.Second,
		Interval: 100 * time.Millisecond,
	}
	done := make(chan struct{})
	if err := adapter.WaitForReady(context.Background(), cfg, done); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestWaitForReady_TCP_Timeout(t *testing.T) {
	cfg := adapter.ReadinessConfig{
		TCP:      "127.0.0.1:1",
		Timeout:  500 * time.Millisecond,
		Interval: 100 * time.Millisecond,
	}
	done := make(chan struct{})
	err := adapter.WaitForReady(context.Background(), cfg, done)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForReady_Cmd_Success(t *testing.T) {
	cfg := adapter.ReadinessConfig{
		Cmd:      "true",
		Timeout:  5 * time.Second,
		Interval: 100 * time.Millisecond,
	}
	done := make(chan struct{})
	if err := adapter.WaitForReady(context.Background(), cfg, done); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestWaitForReady_Cmd_Timeout(t *testing.T) {
	cfg := adapter.ReadinessConfig{
		Cmd:      "false",
		Timeout:  500 * time.Millisecond,
		Interval: 100 * time.Millisecond,
	}
	done := make(chan struct{})
	err := adapter.WaitForReady(context.Background(), cfg, done)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForReady_ProcessCrash(t *testing.T) {
	cfg := adapter.ReadinessConfig{
		HTTP:     "http://127.0.0.1:1/no-server-here",
		Timeout:  10 * time.Second,
		Interval: 100 * time.Millisecond,
	}
	done := make(chan struct{})
	close(done)

	err := adapter.WaitForReady(context.Background(), cfg, done)
	if err == nil {
		t.Fatal("expected error when process exits early")
	}
}

func TestWaitForReady_NoProbe(t *testing.T) {
	cfg := adapter.ReadinessConfig{
		Timeout:  500 * time.Millisecond,
		Interval: 100 * time.Millisecond,
	}
	done := make(chan struct{})
	err := adapter.WaitForReady(context.Background(), cfg, done)
	if err == nil {
		t.Fatal("expected error when no probe type configured")
	}
}
