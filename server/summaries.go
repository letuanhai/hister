// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/indexer"
	"github.com/asciimoo/hister/server/summarizer"
	"github.com/asciimoo/hister/server/types"

	"github.com/rs/zerolog/log"
)

// newSummarizer is replaced in tests.
var newSummarizer = summarizer.New

var backgroundSummaries = newBackgroundQueue("summaries")

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
	idx, u, uid := c.Indexer, d.URL, d.UserID
	backgroundSummaries.enqueue(document.GetDocID(uid, u), cfg.Concurrency, func() {
		if err := summarizeDocument(context.Background(), idx, cfg, u, uid, false); err != nil {
			log.Warn().Err(err).Str("url", u).Msg("background LLM summary failed")
		}
	})
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
