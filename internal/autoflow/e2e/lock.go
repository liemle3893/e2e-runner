package e2e

import (
	"errors"
	"os"
)

// ErrLockTimeout is returned by Acquire when the timeout elapses before
// the lock could be taken.
var ErrLockTimeout = errors.New("e2e lock timeout")

// Lock is a released-on-close file lock, suitable for serialising E2E
// runs that share the main repository directory.
type Lock struct {
	f *os.File
}

// Release unlocks and closes the handle. Idempotent.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	unlock(l.f)
	err := l.f.Close()
	l.f = nil
	return err
}
