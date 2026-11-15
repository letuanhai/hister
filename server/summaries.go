// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/indexer"
	"github.com/asciimoo/hister/server/summarizer"
	"github.com/asciimoo/hister/server/types"

	"github.com/rs/zerolog/log"
)

const summaryQueueSize = 1000

// newSummarizer is replaced in tests.
var newSummarizer = summarizer.New

type summaryJob struct {
	url    string
	userID uint
	cfg    config.Summarizer
	idx    *indexer.Indexer
}

// summaryQueue runs background LLM summaries of newly indexed pages.
type summaryQueue struct {
	once    sync.Once
	jobs    chan summaryJob
	mu      sync.Mutex
	workers int
	pending map[string]struct{}
}

var backgroundSummaries = &summaryQueue{}

func (q *summaryQueue) enqueue(job summaryJob) {
	q.once.Do(func() {
		q.jobs = make(chan summaryJob, summaryQueueSize)
		q.pending = make(map[string]struct{})
	})
	key := document.GetDocID(job.userID, job.url)
	q.mu.Lock()
	if _, queued := q.pending[key]; queued {
		q.mu.Unlock()
		return
	}
	for q.workers < max(1, job.cfg.Concurrency) {
		q.workers++
		go q.work()
	}
	select {
	case q.jobs <- job:
		q.pending[key] = struct{}{}
	default:
		log.Warn().Str("url", job.url).Msg("summary queue is full; skipping LLM summary")
	}
	q.mu.Unlock()
}

func (q *summaryQueue) work() {
	for job := range q.jobs {
		if err := summarizeDocument(context.Background(), job.idx, job.cfg, job.url, job.userID, false); err != nil {
			log.Warn().Err(err).Str("url", job.url).Msg("background LLM summary failed")
		}
		q.mu.Lock()
		delete(q.pending, document.GetDocID(job.userID, job.url))
		q.mu.Unlock()
	}
}

// queueAutoSummary schedules a background summary for a submitted web page
// when automatic LLM summaries are enabled and the page has none yet.
func queueAutoSummary(c *webContext, d *document.Document) {
	cfg := c.Config.Summarizer
	if !cfg.Enable || !cfg.Auto || d == nil || d.Type != document.Web || d.SkipIndexing {
		return
	}
	if d.SummarySource == document.SummarySourceLLM || d.IsUserEdited(document.UserEditedSummary) {
		return
	}
	backgroundSummaries.enqueue(summaryJob{url: d.URL, userID: d.UserID, cfg: cfg, idx: c.Indexer})
}

// summarizeDocument asks the configured model for a summary and stores it.
func summarizeDocument(ctx context.Context, idx *indexer.Indexer, cfg config.Summarizer, u string, userID uint, replaceManual bool) error {
	s, err := newSummarizer(cfg)
	if err != nil {
		return err
	}
	d := idx.GetByURLAndUser(u, userID)
	if d == nil || d.UserID != userID {
		return errors.New("document not found")
	}
	if !replaceManual && d.IsUserEdited(document.UserEditedSummary) {
		return nil
	}
	text := d.Text
	if text == "" {
		text = d.Summary
	}
	if text == "" {
		return errors.New("the document has no text to summarize")
	}
	summary, err := s.Summarize(ctx, summarizer.Input{URL: d.URL, Title: d.Title, Text: text})
	if err != nil {
		return err
	}
	owner := userID
	_, err = idx.UpdateByQuery(documentURLQuery(d.URL), &owner, types.DocumentChanges{
		GeneratedSummary:     &summary,
		ReplaceManualSummary: replaceManual,
	}, false)
	return err
}

// serveDocumentSummarize writes an LLM summary for one document on request.
// It replaces a manual summary because the user asked for it explicitly.
func serveDocumentSummarize(c *webContext) {
	var req struct {
		URL        string `json:"url"`
		DocumentID string `json:"document_id"`
	}
	if !decodeRequestJSON(c, &req) {
		return
	}
	if !c.Config.Summarizer.Enable {
		http.Error(c.Response, summarizer.ErrDisabled.Error(), http.StatusConflict)
		return
	}
	q := c.Request.URL.Query()
	q.Set("document_id", req.DocumentID)
	c.Request.URL.RawQuery = q.Encode()
	doc := requestedDocument(c, req.URL)
	if doc == nil {
		http.Error(c.Response, "document not found", http.StatusNotFound)
		return
	}
	if c.Config.App.UserHandling && !c.IsAdmin && doc.UserID != c.UserID {
		http.Error(c.Response, "only the owner can edit this document", http.StatusForbidden)
		return
	}
	if err := summarizeDocument(c.Request.Context(), c.Indexer, c.Config.Summarizer, doc.URL, doc.UserID, true); err != nil {
		log.Warn().Err(err).Str("url", doc.URL).Msg("LLM summary failed")
		http.Error(c.Response, err.Error(), http.StatusBadGateway)
		return
	}
	updated := c.Indexer.GetByURLAndUser(doc.URL, doc.UserID)
	if updated == nil {
		serve500(c)
		return
	}
	c.JSON(newEditableDocument(updated))
}
