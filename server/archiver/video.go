// SPDX-License-Identifier: AGPL-3.0-or-later

package archiver

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const videoTimeout = 2 * time.Hour

// downloadVideo saves the page's video with yt-dlp. It returns nil without an
// error when yt-dlp does not support the page.
func (a *Archiver) downloadVideo(ctx context.Context, req Request) (*Video, error) {
	ctx, cancel := context.WithTimeout(ctx, videoTimeout)
	defer cancel()
	bin := a.cfg.YtDlpPath
	if bin == "" {
		bin = "yt-dlp"
	}
	args := []string{
		"--no-playlist", "--no-progress", "--no-warnings", "--quiet",
		"--restrict-filenames", "--no-mtime",
		"-o", filepath.Join(req.OutputDir, "video.%(ext)s"),
	}
	if a.cfg.VideoFormat != "" {
		args = append(args, "-f", a.cfg.VideoFormat)
	}
	if a.cfg.MaxVideoBytes > 0 {
		args = append(args, "--max-filesize", strconv.Itoa(a.cfg.MaxVideoBytes))
	}
	if req.CookiesFile != "" {
		args = append(args, "--cookies", req.CookiesFile)
	}
	args = append(args, "--", req.URL)
	cmd := exec.CommandContext(ctx, bin, args...)
	killProcessTree(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedBuffer{buf: &stderr, max: 64 << 10}
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(msg, "Unsupported URL") || strings.Contains(msg, "No video formats found") {
			return nil, nil
		}
		return nil, fmt.Errorf("yt-dlp failed: %s", strings.TrimSpace(msg))
	}
	matches, _ := filepath.Glob(filepath.Join(req.OutputDir, "video.*"))
	for _, m := range matches {
		ext := filepath.Ext(m)
		if ext == ".part" || ext == ".ytdl" {
			continue
		}
		return &Video{File: filepath.Base(m), Type: mime.TypeByExtension(ext)}, nil
	}
	// yt-dlp skips files above --max-filesize without failing.
	return nil, nil
}
