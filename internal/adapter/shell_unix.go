//go:build !windows

package adapter

import (
	"os/exec"
	"syscall"
)

// setProcessGroup places the command in its own process group so the whole tree
// can be signalled together.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup terminates the command and every process it spawned.
//
// Killing only the direct child would leave the children of `sh -c` running:
// a backgrounded server or a `docker run` started by the command would survive
// its own test and interfere with later ones.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}
