//go:build windows

package cli

import "os/exec"

// setProcessGroup is a no-op on Windows: syscall.SysProcAttr has no
// Setpgid field there, and creating an equivalent (a Job Object) is more
// than this needs right now. See dev_unix.go for the platform that has
// one, and stopProcessGroup below for the resulting limitation.
func setProcessGroup(c *exec.Cmd) {}

// stopProcessGroup kills c's direct child process only. Unlike
// dev_unix.go's implementation, this does not reach further descendants
// (e.g. the actual vite process a `npm run dev` execs into) — a known gap
// versus Unix, not a full equivalent.
func stopProcessGroup(c *exec.Cmd) error {
	return c.Process.Kill()
}
