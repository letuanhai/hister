// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build windows

package archiver

import (
	"os/exec"
	"time"
)

// killProcessTree bounds waiting for output after cancellation. Windows has
// no process groups here; the child is killed on its own.
func killProcessTree(cmd *exec.Cmd) {
	cmd.WaitDelay = 5 * time.Second
}
