// SPDX-License-Identifier: AGPL-3.0-or-later

package document

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxAutoSummaryLength bounds automatic summaries in bytes.
	MaxAutoSummaryLength = 400
	// minAutoSummaryLength skips descriptions too short to be useful.
	minAutoSummaryLength = 40
)

// EnsureAutoSummary fills an empty summary from the page description or,
// failing that, from the opening sentences of the extracted text.
func (d *Document) EnsureAutoSummary() {
	if strings.TrimSpace(d.Summary) != "" {
		if d.SummarySource == "" {
			d.SummarySource = SummarySourceAuto
		}
		return
	}
	summary := AutoSummary(d.Title, d.metadataString("description"), d.Text)
	if summary == "" {
		return
	}
	d.Summary = summary
	d.SummarySource = SummarySourceAuto
}

func (d *Document) metadataString(key string) string {
	s, _ := d.Metadata[key].(string)
	return s
}

// AutoSummary picks a short summary: the description when it is useful,
// otherwise the opening sentences of text.
func AutoSummary(title, description, text string) string {
	description = collapseSpace(description)
	if len(description) >= minAutoSummaryLength && !strings.EqualFold(description, collapseSpace(title)) {
		return truncateAtWord(description, MaxAutoSummaryLength)
	}
	return leadSentences(text, title)
}

// leadSentences returns the first sentences of the first paragraph that
// looks like prose, so navigation fragments and headings are skipped.
func leadSentences(text, title string) string {
	title = collapseSpace(title)
	for _, paragraph := range strings.Split(text, "\n") {
		p := collapseSpace(paragraph)
		if len(p) < minAutoSummaryLength || strings.EqualFold(p, title) || !strings.ContainsAny(p, ".!?。") {
			continue
		}
		if len(p) <= MaxAutoSummaryLength {
			return p
		}
		// Cut at the last sentence end within the limit when there is one.
		cut := p[:MaxAutoSummaryLength]
		if i := strings.LastIndexAny(cut, ".!?。"); i >= minAutoSummaryLength {
			return cut[:i+1]
		}
		return truncateAtWord(p, MaxAutoSummaryLength)
	}
	return ""
}

func collapseSpace(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

func truncateAtWord(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := s[:limit]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	if i := strings.LastIndexByte(cut, ' '); i > limit/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:-") + "…"
}
