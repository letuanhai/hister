//go:build !windows

package archiver

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/asciimoo/hister/config"
)

func TestSingleFileTimeoutKillsBrowser(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "browser.pid")
	// A stand in for SingleFile that starts a long running "browser" child.
	script := filepath.Join(dir, "single-file")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 300 &\necho $! > "+pidFile+"\nwait\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := New(config.Archiver{SingleFilePath: script, Timeout: 1}, "")
	start := time.Now()
	_, err := a.Archive(context.Background(), Request{URL: "http://127.0.0.1:1/", OutputDir: dir}, Options{Engine: "singlefile"})
	if err == nil {
		t.Fatal("archive did not fail on timeout")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("timeout took %s", time.Since(start))
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("child process %d survived the timeout", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
