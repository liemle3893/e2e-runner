package adapter

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// ReadinessConfig describes a health-check probe used to gate test execution
// until a background process is ready. Exactly one of HTTP, TCP, or Cmd must be set.
type ReadinessConfig struct {
	HTTP     string
	TCP      string
	Cmd      string
	Timeout  time.Duration
	Interval time.Duration
}

// WaitForReady polls the configured probe until it succeeds, the timeout expires,
// or the done channel signals that the process has exited.
func WaitForReady(ctx context.Context, cfg ReadinessConfig, done <-chan struct{}) error {
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Interval == 0 {
		cfg.Interval = 500 * time.Millisecond
	}

	deadline := time.After(cfg.Timeout)
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	var lastErr error

	for {
		select {
		case <-done:
			return fmt.Errorf("process exited before becoming ready (last probe error: %v)", lastErr)
		case <-deadline:
			return tryve.TimeoutError("readiness probe", cfg.Timeout)
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var err error
			switch {
			case cfg.HTTP != "":
				err = probeHTTP(ctx, cfg.HTTP, cfg.Interval)
			case cfg.TCP != "":
				err = probeTCP(cfg.TCP, cfg.Interval)
			case cfg.Cmd != "":
				err = probeCmd(ctx, cfg.Cmd)
			default:
				return fmt.Errorf("readiness: no probe type configured (set http, tcp, or cmd)")
			}
			if err == nil {
				return nil
			}
			lastErr = err
		}
	}
}

// probeHTTP sends a GET request and succeeds on any 2xx status.
func probeHTTP(ctx context.Context, url string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP probe returned status %d", resp.StatusCode)
	}
	return nil
}

// probeTCP dials the address and succeeds when a connection is established.
func probeTCP(addr string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// probeCmd runs a shell command and succeeds when it exits with code 0.
func probeCmd(ctx context.Context, command string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("readiness cmd failed: %w", err)
	}
	return nil
}
