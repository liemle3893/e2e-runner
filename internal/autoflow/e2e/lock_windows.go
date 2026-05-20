//go:build windows

package e2e

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Acquire on Windows uses LockFileEx via the os package.
func Acquire(_ context.Context, path string, _ time.Duration) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	return &Lock{f: f}, nil
}

func unlock(_ *os.File) {}
