// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/archiver"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/indexer"
	"github.com/asciimoo/hister/server/model"

	"github.com/rs/zerolog/log"
)

const (
	archivesDirName   = "archives"
	defaultUserAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	archiveCSP        = "default-src 'none'; img-src data:; media-src 'self'; style-src 'unsafe-inline'; sandbox allow-same-origin allow-popups allow-popups-to-escape-sandbox"
	maxArchivesPerDoc = 50
)

var backgroundArchives = newBackgroundQueue("archives")

type archiveRequest struct {
	URL        string `json:"url"`
	DocumentID string `json:"document_id"`
	Engine     string `json:"engine"`
	KeepImages *bool  `json:"keep_images"`
	Mobile     *bool  `json:"mobile"`
	Adblock    *bool  `json:"adblock"`
	Video      *bool  `json:"video"`
}

func (r archiveRequest) options(cfg config.Archiver) archiver.Options {
	opts := archiver.DefaultOptions(cfg)
	if r.Engine != "" {
		opts.Engine = r.Engine
	}
	for _, o := range []struct {
		src *bool
		dst *bool
	}{{r.KeepImages, &opts.KeepImages}, {r.Mobile, &opts.Mobile}, {r.Adblock, &opts.Adblock}, {r.Video, &opts.Video}} {
		if o.src != nil {
			*o.dst = *o.src
		}
	}
	return opts
}

func archivesRoot(idx *indexer.Indexer) string {
	return filepath.Join(idx.DataDir(), archivesDirName)
}

// cookiesPath is where a user's uploaded cookies file is kept.
func cookiesPath(idx *indexer.Indexer, userID uint) string {
	return filepath.Join(archivesRoot(idx), "cookies", fmt.Sprintf("user-%d.txt", userID))
}

// profilePath is a user's persistent browser profile, or empty when profiles are off.
func profilePath(cfg config.Archiver, userID uint) string {
	return archiver.UserProfileDir(cfg.ProfileDir, userID)
}

func archiverUserAgent(cfg *config.Config) string {
	if cfg.Crawler.UserAgent != "" {
		return cfg.Crawler.UserAgent
	}
	return defaultUserAgent
}

// createArchive archives a document and records the result. Video downloads
// continue in the background after the page archive is saved.
func createArchive(ctx context.Context, cfg *config.Config, idx *indexer.Indexer, doc *document.Document, opts archiver.Options) (*model.DocumentArchive, error) {
	rec := &model.DocumentArchive{
		URL:    doc.URL,
		UserID: doc.UserID,
		Title:  doc.Title,
		Engine: opts.Engine,
		Status: model.ArchivePending,
		Images: opts.KeepImages,
		Mobile: opts.Mobile,
	}
	if err := model.CreateArchive(rec); err != nil {
		return nil, err
	}
	rec.Dir = filepath.Join(archivesRoot(idx), strconv.FormatUint(uint64(rec.ID), 10))
	if err := os.MkdirAll(rec.Dir, 0o700); err != nil {
		return nil, err
	}
	req := archiver.Request{
		URL:        doc.URL,
		StoredHTML: doc.HTML,
		Title:      doc.Title,
		Language:   doc.Language,
		ProfileDir: profilePath(cfg.Archiver, doc.UserID),
		OutputDir:  rec.Dir,
	}
	if cookies := cookiesPath(idx, doc.UserID); fileExists(cookies) {
		req.CookiesFile = cookies
	}
	a := archiver.New(cfg.Archiver, archiverUserAgent(cfg))
	wantVideo := opts.Video
	opts.Video = false
	res, err := a.Archive(ctx, req, opts)
	if err != nil {
		rec.Status, rec.Error = model.ArchiveFailed, err.Error()
		_ = os.RemoveAll(rec.Dir)
		if saveErr := model.SaveArchive(rec); saveErr != nil {
			log.Warn().Err(saveErr).Msg("failed to record archive failure")
		}
		return rec, err
	}
	rec.Status, rec.Title, rec.Size = model.ArchiveReady, res.Title, res.Size
	if wantVideo {
		rec.VideoStatus = model.ArchivePending
	}
	if err := model.SaveArchive(rec); err != nil {
		return nil, err
	}
	pruneArchives(doc.URL, doc.UserID)
	if wantVideo {
		go addArchiveVideo(a, req, rec.ID, rec.UserID)
	}
	return rec, nil
}

func addArchiveVideo(a *archiver.Archiver, req archiver.Request, id, userID uint) {
	v, err := a.AddVideo(context.Background(), req)
	rec, getErr := model.GetArchive(id, userID)
	if getErr != nil {
		return
	}
	switch {
	case err != nil:
		log.Warn().Err(err).Str("url", req.URL).Msg("archive video download failed")
		rec.VideoStatus = model.ArchiveFailed
	case v == nil:
		rec.VideoStatus = "none"
	default:
		rec.VideoStatus, rec.VideoFile = model.ArchiveReady, v.File
		if fi, err := os.Stat(filepath.Join(req.OutputDir, v.File)); err == nil {
			rec.Size += fi.Size()
		}
	}
	if err := model.SaveArchive(rec); err != nil {
		log.Warn().Err(err).Msg("failed to record archive video")
	}
}

// pruneArchives keeps the newest archives of a document.
func pruneArchives(u string, userID uint) {
	archives, err := model.GetArchives(u, userID)
	if err != nil || len(archives) <= maxArchivesPerDoc {
		return
	}
	for _, a := range archives[maxArchivesPerDoc:] {
		removeArchive(a)
	}
}

func removeArchive(a *model.DocumentArchive) {
	if _, err := model.DeleteArchive(a.ID, a.UserID); err != nil {
		log.Warn().Err(err).Uint("id", a.ID).Msg("failed to delete archive")
		return
	}
	if a.Dir != "" {
		if err := os.RemoveAll(a.Dir); err != nil {
			log.Warn().Err(err).Str("dir", a.Dir).Msg("failed to delete archive files")
		}
	}
}

// deleteDocumentArchives removes every archive of a deleted document.
func deleteDocumentArchives(u string, userID uint) {
	archives, err := model.DeleteArchives(u, userID)
	if err != nil {
		log.Warn().Err(err).Str("url", u).Msg("failed to delete archives for deleted document")
		return
	}
	for _, a := range archives {
		if a.Dir != "" {
			_ = os.RemoveAll(a.Dir)
		}
	}
}

// queueAutoArchive archives a newly submitted web page in the background.
func queueAutoArchive(c *webContext, d *document.Document) {
	cfg := c.Config
	if !cfg.Archiver.Enable || !cfg.Archiver.Auto || d == nil || d.Type != document.Web || d.SkipIndexing {
		return
	}
	if latest, err := model.GetLatestReadyArchive(d.URL, d.UserID); err == nil && time.Since(latest.CreatedAt) < 24*time.Hour {
		return
	}
	idx, u, uid := c.Indexer, d.URL, d.UserID
	backgroundArchives.enqueue(document.GetDocID(uid, u), cfg.Archiver.Concurrency, func() {
		doc := idx.GetByURLAndUser(u, uid)
		if doc == nil || doc.UserID != uid {
			return
		}
		if _, err := createArchive(context.Background(), cfg, idx, doc, archiver.DefaultOptions(cfg.Archiver)); err != nil {
			log.Warn().Err(err).Str("url", u).Msg("background archive failed")
		}
	})
}

// ownedDocument resolves a document the current user may change.
func ownedDocument(c *webContext, u, documentID string) (*document.Document, int, string) {
	q := c.Request.URL.Query()
	q.Set("document_id", documentID)
	c.Request.URL.RawQuery = q.Encode()
	doc := requestedDocument(c, u)
	if doc == nil {
		return nil, http.StatusNotFound, "document not found"
	}
	if c.Config.App.UserHandling && !c.IsAdmin && doc.UserID != c.UserID {
		return nil, http.StatusForbidden, "only the owner can archive this document"
	}
	return doc, 0, ""
}

func serveCreateArchive(c *webContext) {
	var req archiveRequest
	if !decodeRequestJSON(c, &req) {
		return
	}
	if !c.Config.Archiver.Enable {
		http.Error(c.Response, "archives are disabled", http.StatusConflict)
		return
	}
	switch req.Engine {
	case "", config.ArchiverEngineStored, config.ArchiverEngineBrowser, config.ArchiverEngineSingleFile:
	default:
		http.Error(c.Response, "unknown engine", http.StatusBadRequest)
		return
	}
	doc, status, msg := ownedDocument(c, req.URL, req.DocumentID)
	if doc == nil {
		http.Error(c.Response, msg, status)
		return
	}
	rec, err := createArchive(c.Request.Context(), c.Config, c.Indexer, doc, req.options(c.Config.Archiver))
	if err != nil {
		if rec == nil {
			log.Error().Err(err).Str("url", doc.URL).Msg("archive failed")
			serve500(c)
			return
		}
		status := http.StatusBadGateway
		if errors.Is(err, archiver.ErrNoStoredHTML) {
			status = http.StatusConflict
		}
		c.JSONStatus(status, rec)
		return
	}
	c.JSONStatus(http.StatusCreated, rec)
}

func serveListArchives(c *webContext) {
	q := c.Request.URL.Query()
	if u := q.Get("url"); u != "" {
		doc := requestedDocument(c, u)
		if doc == nil {
			http.Error(c.Response, "document not found", http.StatusNotFound)
			return
		}
		archives, err := model.GetArchives(doc.URL, doc.UserID)
		if err != nil {
			serve500(c)
			return
		}
		if archives == nil {
			archives = []*model.DocumentArchive{}
		}
		c.JSON(map[string]any{"total": len(archives), "archives": archives})
		return
	}
	limit, offset := 100, 0
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = min(n, 1000)
	}
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n > 0 {
		offset = n
	}
	archives, total, err := model.ListReadyArchives(targetUserID(c), limit, offset)
	if err != nil {
		serve500(c)
		return
	}
	if archives == nil {
		archives = []*model.DocumentArchive{}
	}
	c.JSON(map[string]any{"total": total, "archives": archives})
}

// serveArchiveFile serves the page or a media file of an archive with a
// policy that blocks scripts and remote requests.
func serveArchiveFile(c *webContext) {
	id, err := strconv.ParseUint(c.Request.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(c.Response, c.Request)
		return
	}
	rec, err := model.GetArchive(uint(id), targetUserID(c))
	if err != nil && targetUserID(c) != 0 {
		// Global documents belong to user 0.
		rec, err = model.GetArchive(uint(id), 0)
	}
	if err != nil || rec.Status != model.ArchiveReady || rec.Dir == "" {
		http.NotFound(c.Response, c.Request)
		return
	}
	name := c.Request.PathValue("file")
	if name == "" {
		name = archiver.IndexFile
	}
	if name != archiver.IndexFile && (rec.VideoFile == "" || name != rec.VideoFile) {
		http.NotFound(c.Response, c.Request)
		return
	}
	f, err := os.Open(filepath.Join(rec.Dir, name))
	if err != nil {
		http.NotFound(c.Response, c.Request)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		serve500(c)
		return
	}
	h := c.Response.Header()
	h.Set("Content-Security-Policy", archiveCSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	if name == archiver.IndexFile {
		h.Set("Content-Type", "text/html; charset=utf-8")
	} else if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		h.Set("Content-Type", t)
	}
	http.ServeContent(c.Response, c.Request, name, fi.ModTime(), f)
}

func serveDeleteArchive(c *webContext) {
	var req struct {
		ID uint `json:"id"`
	}
	if !decodeRequestJSON(c, &req) {
		return
	}
	rec, err := model.GetArchive(req.ID, targetUserID(c))
	if err != nil {
		http.Error(c.Response, "archive not found", http.StatusNotFound)
		return
	}
	removeArchive(rec)
	c.Response.WriteHeader(http.StatusNoContent)
}

type archiverStatus struct {
	Enabled    bool   `json:"enabled"`
	Engine     string `json:"engine"`
	KeepImages bool   `json:"keep_images"`
	Mobile     bool   `json:"mobile"`
	Adblock    bool   `json:"adblock"`
	Video      bool   `json:"video"`
	HasCookies bool   `json:"has_cookies"`
	HasProfile bool   `json:"has_profile"`
}

func serveArchiverStatus(c *webContext) {
	cfg := c.Config.Archiver
	uid := targetUserID(c)
	c.JSON(archiverStatus{
		Enabled:    cfg.Enable,
		Engine:     cfg.Engine,
		KeepImages: cfg.KeepImages,
		Mobile:     cfg.Mobile,
		Adblock:    cfg.Adblock,
		Video:      cfg.Video,
		HasCookies: fileExists(cookiesPath(c.Indexer, uid)),
		HasProfile: profilePath(cfg, uid) != "" && fileExists(profilePath(cfg, uid)),
	})
}

// serveArchiverCookies stores the uploaded cookies file of the current user.
// An empty body removes it.
func serveArchiverCookies(c *webContext) {
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, archiver.MaxCookiesFileBytes+1))
	if err != nil {
		http.Error(c.Response, "invalid body", http.StatusBadRequest)
		return
	}
	path := cookiesPath(c.Indexer, targetUserID(c))
	if strings.TrimSpace(string(data)) == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			serve500(c)
			return
		}
		c.Response.WriteHeader(http.StatusNoContent)
		return
	}
	cookies, err := archiver.ParseCookies(strings.NewReader(string(data)))
	if err != nil {
		http.Error(c.Response, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		serve500(c)
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		serve500(c)
		return
	}
	c.JSON(map[string]int{"cookies": len(cookies)})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
