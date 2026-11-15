// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/indexer"

	"github.com/rs/zerolog/log"
)

const (
	defaultTagSuggestions = 20
	maxTagSuggestions     = 200
	// tagScanSize bounds how many distinct tags are aggregated before the
	// prefix filter is applied.
	tagScanSize = 5000
)

type tagSuggestion struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// serveTags lists the current user's tags ordered by use, optionally limited
// to tags starting with or containing a prefix. It powers tag autocompletion.
func serveTags(c *webContext) {
	params := c.Request.URL.Query()
	prefix := document.NormalizeTag(params.Get("prefix"))
	limit := defaultTagSuggestions
	if s := params.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			http.Error(c.Response, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = min(n, maxTagSuggestions)
	}
	q := &indexer.Query{
		MatchAll:   true,
		Facets:     true,
		FacetsOnly: true,
		FacetSizes: map[string]int{"tags": tagScanSize},
	}
	res, err := searchIndex(c.Indexer, q, nil, c.UserID)
	if err != nil {
		log.Error().Err(err).Msg("failed to aggregate tags")
		serve500(c)
		return
	}
	var prefixed, contained []tagSuggestion
	if res.Facets != nil {
		for _, term := range res.Facets.Terms["tags"].Terms {
			switch {
			case prefix == "" || strings.HasPrefix(term.Term, prefix):
				prefixed = append(prefixed, tagSuggestion{Tag: term.Term, Count: term.Count})
			case strings.Contains(term.Term, prefix):
				contained = append(contained, tagSuggestion{Tag: term.Term, Count: term.Count})
			}
		}
	}
	out := append(prefixed, contained...)
	if len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []tagSuggestion{}
	}
	c.JSON(out)
}
