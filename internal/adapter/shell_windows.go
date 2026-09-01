//go:build windows

package adapter

import "os/exec"

// setProcessGroup is a no-op on Windows, where process groups are not created
// through SysProcAttr in the same way.
func setProcessGroup(_ *exec.Cmd) {}

// killProcessGroup terminates the command process.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
