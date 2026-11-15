// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/model"

	"github.com/rs/zerolog/log"
)

const (
	defaultVisitsLimit = 100
	maxVisitsLimit     = 1000
)

// visitTimestamp is an optional request field carrying the time a page was
// visited as a Unix timestamp. History importers use it to keep the original
// visit time; live submissions omit it and record the current time.
type visitTimestamp struct {
	Visited int64 `json:"visited,omitempty"`
}

func (v visitTimestamp) time() time.Time {
	if v.Visited <= 0 {
		return time.Time{}
	}
	t := time.Unix(v.Visited, 0)
	// Visits cannot happen in the future; clamp clock skew to now.
	if t.After(time.Now()) {
		return time.Time{}
	}
	return t
}

type pendingVisit struct {
	doc *document.Document
	at  time.Time
}

type visitsResponse struct {
	Total  int64                  `json:"total"`
	Visits []*model.DocumentVisit `json:"visits"`
}

// recordDocumentVisit stores a visit for a successfully submitted web
// document. Local and remote file snapshots are not visits.
func recordDocumentVisit(d *document.Document, at time.Time) {
	if d == nil || d.SkipIndexing || d.Type != document.Web {
		return
	}
	if _, err := model.RecordVisit(d.URL, d.UserID, at); err != nil {
		log.Warn().Err(err).Str("url", d.URL).Msg("failed to record document visit")
	}
}

// deleteDocumentRecords removes the database records tied to a deleted
// document: search history entries, recorded visits and archives.
func deleteDocumentRecords(url string, uid uint) {
	if err := model.DeleteHistoryURL(uid, url); err != nil {
		log.Warn().Err(err).Str("url", url).Msg("failed to delete history for deleted document")
	}
	if err := model.DeleteVisits(url, uid); err != nil {
		log.Warn().Err(err).Str("url", url).Msg("failed to delete visits for deleted document")
	}
	deleteDocumentArchives(url, uid)
}

func serveVisits(c *webContext) {
	q := c.Request.URL.Query()
	u := q.Get("url")
	if u == "" {
		http.Error(c.Response, "url parameter is required", http.StatusBadRequest)
		return
	}
	doc := requestedDocument(c, u)
	if doc == nil {
		http.Error(c.Response, "document not found", http.StatusNotFound)
		return
	}
	limit := defaultVisitsLimit
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			http.Error(c.Response, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = min(n, maxVisitsLimit)
	}
	offset := 0
	if s := q.Get("offset"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			http.Error(c.Response, "invalid offset", http.StatusBadRequest)
			return
		}
		offset = n
	}
	total, err := model.CountVisits(doc.URL, doc.UserID)
	if err != nil {
		log.Error().Err(err).Str("url", u).Msg("failed to count document visits")
		serve500(c)
		return
	}
	visits, err := model.GetVisits(doc.URL, doc.UserID, limit, offset)
	if err != nil {
		log.Error().Err(err).Str("url", u).Msg("failed to get document visits")
		serve500(c)
		return
	}
	c.JSON(visitsResponse{Total: total, Visits: visits})
}

func serveDeleteVisit(c *webContext) {
	var req struct {
		ID uint `json:"id"`
	}
	if !decodeRequestJSON(c, &req) {
		return
	}
	if req.ID == 0 {
		http.Error(c.Response, "missing id", http.StatusBadRequest)
		return
	}
	deleted, err := model.DeleteVisit(req.ID, targetUserID(c))
	if err != nil {
		log.Error().Err(err).Uint("id", req.ID).Msg("failed to delete document visit")
		serve500(c)
		return
	}
	if !deleted {
		http.Error(c.Response, "visit not found", http.StatusNotFound)
		return
	}
	c.Response.WriteHeader(http.StatusNoContent)
}
