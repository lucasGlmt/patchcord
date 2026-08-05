//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// setProcessGroup configures c to run in its own process group so that
// stopProcessGroup below can signal the whole tree it starts (e.g. the
// actual vite process a `npm run dev` execs into), not just c's direct
// PID. See dev_windows.go for the platform without a POSIX process group.
func setProcessGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// stopProcessGroup sends SIGTERM to the whole process group started by
// setProcessGroup — a negative pid targets the group, not just
// c.Process itself.
func stopProcessGroup(c *exec.Cmd) error {
	return syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
}
