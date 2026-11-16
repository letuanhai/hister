// SPDX-License-Identifier: AGPL-3.0-or-later

package archiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const maxSingleFileOutput = 512 << 20

// fetchWithSingleFile saves a page with the SingleFile CLI and returns the
// self contained HTML, with resources such as images embedded as data URIs.
func (a *Archiver) fetchWithSingleFile(ctx context.Context, req Request, opts Options) (string, error) {
	unlock := lockProfile(req.ProfileDir)
	defer unlock()
	args := a.singleFileArgs(req, opts)
	cmd := exec.CommandContext(ctx, a.singleFilePath(), args...)
	killProcessTree(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedBuffer{buf: &stdout, max: maxSingleFileOutput}
	cmd.Stderr = &limitedBuffer{buf: &stderr, max: 64 << 10}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("singlefile failed: %s", msg)
	}
	out := stdout.String()
	if !strings.Contains(strings.ToLower(out[:min(len(out), 4096)]), "<html") {
		return "", fmt.Errorf("singlefile returned no HTML: %q %q", out[:min(len(out), 300)], strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (a *Archiver) singleFilePath() string {
	if a.cfg.SingleFilePath != "" {
		return a.cfg.SingleFilePath
	}
	return "single-file"
}

func (a *Archiver) singleFileArgs(req Request, opts Options) []string {
	browserArgs := []string{}
	if os.Geteuid() == 0 {
		browserArgs = append(browserArgs, "--no-sandbox")
	}
	if req.ProfileDir != "" {
		browserArgs = append(browserArgs, "--user-data-dir="+req.ProfileDir)
	}
	if opts.Adblock && a.cfg.AdblockExtension != "" {
		browserArgs = append(browserArgs,
			"--disable-extensions-except="+a.cfg.AdblockExtension,
			"--load-extension="+a.cfg.AdblockExtension)
	}
	encoded, _ := json.Marshal(browserArgs)
	args := []string{
		req.URL,
		"--dump-content",
		"--browser-args=" + string(encoded),
		"--browser-load-max-time=" + strconv.Itoa(a.cfg.Timeout*1000),
		"--block-scripts=true",
		"--block-videos=true",
		"--block-audios=true",
		"--remove-hidden-elements=true",
		"--remove-unused-styles=true",
		"--load-deferred-images=true",
		"--compress-content=false",
		"--include-infobar=false",
		"--max-resource-size-enabled=true",
		"--max-resource-size=" + strconv.Itoa(max(1, a.cfg.MaxImageBytes>>20)),
	}
	if a.cfg.BrowserPath != "" {
		args = append(args, "--browser-executable-path="+a.cfg.BrowserPath)
	}
	if !opts.KeepImages {
		args = append(args, "--block-images=true")
	}
	if opts.Mobile {
		args = append(args, "--browser-mobile-emulation=true", "--user-agent="+MobileUserAgent)
	} else if a.userAgent != "" {
		args = append(args, "--user-agent="+a.userAgent)
	}
	if req.CookiesFile != "" {
		args = append(args, "--browser-cookies-file="+req.CookiesFile)
	}
	return args
}

// limitedBuffer stops collecting output after max bytes.
type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		l.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
