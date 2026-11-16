// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !windows

package archiver

import (
	"os/exec"
	"syscall"
	"time"
)

// killProcessTree makes cancellation stop cmd and everything it started, such
// as the browser SingleFile launches. Otherwise a killed run can leave a
// browser behind that keeps the user's profile locked.
func killProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
}
