// SPDX-License-Identifier: AGPL-3.0-or-later

// Package archiver saves reader view archives of web pages: the main text
// content with embedded images, optionally fetched through a headless
// browser or SingleFile, for offline reading.
package archiver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/asciimoo/hister/config"

	"github.com/rs/zerolog/log"
)

// IndexFile is the archive page inside an archive directory.
const IndexFile = "index.html"

// MobileUserAgent is sent when the mobile version of a page is requested.
const MobileUserAgent = "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36"

// ErrNoStoredHTML is returned by the stored engine when a document has no captured HTML.
var ErrNoStoredHTML = errors.New("the document has no stored HTML; use the browser or singlefile engine")

// Options select how a page is archived. Zero values come from the configuration.
type Options struct {
	Engine     string
	KeepImages bool
	Mobile     bool
	Adblock    bool
	Video      bool
}

// DefaultOptions returns the options configured for an archiver.
func DefaultOptions(cfg config.Archiver) Options {
	return Options{
		Engine:     cfg.Engine,
		KeepImages: cfg.KeepImages,
		Mobile:     cfg.Mobile,
		Adblock:    cfg.Adblock,
		Video:      cfg.Video,
	}
}

// Request describes one page to archive.
type Request struct {
	URL        string
	StoredHTML string
	Title      string
	Language   string
	// ProfileDir is the user's persistent browser profile, if any.
	ProfileDir string
	// CookiesFile is a Netscape or JSON cookies file sent with requests.
	CookiesFile string
	// OutputDir receives index.html and media files. It must exist.
	OutputDir string
}

// Result describes a saved archive.
type Result struct {
	Title  string
	Engine string
	Size   int64
	Videos []Video
}

// Archiver creates archives with one configuration.
type Archiver struct {
	cfg       config.Archiver
	client    *http.Client
	userAgent string
	blocklist blocklistCache
}

// New creates an archiver. userAgent is used for image and filter list requests.
func New(cfg config.Archiver, userAgent string) *Archiver {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120
	}
	if cfg.MaxImageBytes <= 0 {
		cfg.MaxImageBytes = 5 << 20
	}
	return &Archiver{
		cfg:       cfg,
		client:    &http.Client{Timeout: 30 * time.Second},
		userAgent: userAgent,
	}
}

func (a *Archiver) blocklistFor(ctx context.Context, opts Options) *Blocklist {
	if !opts.Adblock || len(a.cfg.AdblockLists) == 0 {
		return nil
	}
	return a.blocklist.get(ctx, a.cfg.AdblockLists, a.client)
}

// Archive fetches a page with the selected engine, extracts its main text,
// embeds or removes images, downloads its video when requested and writes
// the reader page to req.OutputDir.
func (a *Archiver) Archive(ctx context.Context, req Request, opts Options) (*Result, error) {
	if opts.Engine == "" {
		opts.Engine = config.ArchiverEngineStored
	}
	pageCtx, cancel := context.WithTimeout(ctx, time.Duration(a.cfg.Timeout)*time.Second)
	defer cancel()
	blocklist := a.blocklistFor(pageCtx, opts)

	var pageHTML string
	var err error
	switch opts.Engine {
	case config.ArchiverEngineStored:
		if req.StoredHTML == "" {
			return nil, ErrNoStoredHTML
		}
		pageHTML = req.StoredHTML
	case config.ArchiverEngineBrowser:
		pageHTML, err = a.fetchWithBrowser(pageCtx, req, opts, blocklist)
	case config.ArchiverEngineSingleFile:
		pageHTML, err = a.fetchWithSingleFile(pageCtx, req, opts)
	default:
		return nil, fmt.Errorf("unknown archiver engine %q", opts.Engine)
	}
	if err != nil {
		return nil, err
	}

	if err := pageCtx.Err(); err != nil {
		return nil, err
	}
	art, err := ExtractArticle(pageHTML, req.URL)
	if err != nil {
		return nil, err
	}
	if art.Title == "" {
		art.Title = req.Title
	}
	if art.Title == "" {
		art.Title = req.URL
	}
	userAgent := a.userAgent
	if opts.Mobile {
		userAgent = MobileUserAgent
	}
	inliner := &imageInliner{
		client:    a.client,
		userAgent: userAgent,
		maxBytes:  int64(a.cfg.MaxImageBytes),
		blocklist: blocklist,
		cache:     make(map[string]string),
	}
	content, err := processImages(pageCtx, art.Content, opts.KeepImages, inliner)
	if err != nil {
		return nil, err
	}
	art.Content = readerPolicy.Sanitize(content)

	var videos []Video
	if opts.Video {
		// Videos can take far longer than a page; they use the caller's context.
		if v, err := a.downloadVideo(ctx, req); err != nil {
			log.Warn().Err(err).Str("url", req.URL).Msg("video download failed")
		} else if v != nil {
			videos = append(videos, *v)
		}
	}

	page, err := RenderReader(art, req.URL, req.Language, time.Now(), videos)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(req.OutputDir, IndexFile), page, 0o600); err != nil {
		return nil, err
	}
	size := int64(len(page))
	for _, v := range videos {
		if fi, err := os.Stat(filepath.Join(req.OutputDir, v.File)); err == nil {
			size += fi.Size()
		}
	}
	return &Result{Title: art.Title, Engine: opts.Engine, Size: size, Videos: videos}, nil
}

func logWarn(err error, msg, source string) {
	log.Warn().Err(err).Str("source", source).Msg(msg)
}

// AddVideo downloads the page's video into an existing archive and adds a
// player to its page. It returns nil without an error when the page has no
// supported video.
func (a *Archiver) AddVideo(ctx context.Context, req Request) (*Video, error) {
	v, err := a.downloadVideo(ctx, req)
	if err != nil || v == nil {
		return v, err
	}
	index := filepath.Join(req.OutputDir, IndexFile)
	page, err := os.ReadFile(index)
	if err != nil {
		return nil, err
	}
	player := fmt.Sprintf("<article>\n<video controls preload=\"metadata\" src=\"%s\"></video>\n", v.File)
	page = bytes.Replace(page, []byte("<article>\n"), []byte(player), 1)
	if err := os.WriteFile(index, page, 0o600); err != nil {
		return nil, err
	}
	return v, nil
}

// UserProfileDir is the persistent browser profile of a user under base, or
// empty when base is empty.
func UserProfileDir(base string, userID uint) string {
	if base == "" {
		return ""
	}
	return filepath.Join(base, fmt.Sprintf("user-%d", userID))
}
