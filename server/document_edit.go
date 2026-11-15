// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/indexer"
	"github.com/asciimoo/hister/server/types"

	"github.com/rs/zerolog/log"
)

type documentEditRequest struct {
	URL        string                `json:"url"`
	DocumentID string                `json:"document_id"`
	Changes    types.DocumentChanges `json:"changes"`
}

// editableDocument is the user editable view of a document returned after an edit.
type editableDocument struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Label         string   `json:"label"`
	Tags          []string `json:"tags"`
	Summary       string   `json:"summary"`
	SummarySource string   `json:"summary_source"`
	Notes         string   `json:"notes"`
	UserEdited    []string `json:"user_edited"`
}

func newEditableDocument(d *document.Document) editableDocument {
	tags := d.Tags
	if tags == nil {
		tags = []string{}
	}
	userEdited := d.UserEdited
	if userEdited == nil {
		userEdited = []string{}
	}
	return editableDocument{
		URL:           d.URL,
		Title:         d.Title,
		Label:         d.Label,
		Tags:          tags,
		Summary:       d.Summary,
		SummarySource: d.SummarySource,
		Notes:         d.Notes,
		UserEdited:    userEdited,
	}
}

// documentURLQuery selects one document by its exact URL.
func documentURLQuery(u string) string {
	return `url:"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(u) + `"`
}

// serveDocumentEdit changes the user editable data of a single document. The
// content and URL of a document cannot be changed.
func serveDocumentEdit(c *webContext) {
	var req documentEditRequest
	if !decodeRequestJSON(c, &req) {
		return
	}
	if req.URL == "" {
		http.Error(c.Response, "missing url", http.StatusBadRequest)
		return
	}
	// Ownership changes go through the bulk update API.
	req.Changes.UserID = nil
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
	owner := doc.UserID
	if _, err := c.Indexer.UpdateByQuery(documentURLQuery(doc.URL), &owner, req.Changes, false); err != nil {
		switch {
		case errors.Is(err, indexer.ErrEmptyUpdate), errors.Is(err, indexer.ErrInvalidLanguage):
			http.Error(c.Response, err.Error(), http.StatusBadRequest)
		default:
			log.Error().Err(err).Str("url", doc.URL).Msg("document edit failed")
			serve500(c)
		}
		return
	}
	updated := c.Indexer.GetByURLAndUser(doc.URL, owner)
	if updated == nil {
		serve500(c)
		return
	}
	c.JSON(newEditableDocument(updated))
}
