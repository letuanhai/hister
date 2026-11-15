// SPDX-License-Identifier: AGPL-3.0-or-later

package document

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Summary sources.
const (
	SummarySourceAuto = "auto"
	SummarySourceLLM  = "llm"
	SummarySourceUser = "user"
)

// User editable fields tracked in Document.UserEdited.
const (
	UserEditedTitle   = "title"
	UserEditedSummary = "summary"
)

// IsUserEdited reports whether the user changed field by hand.
func (d *Document) IsUserEdited(field string) bool {
	return slices.Contains(d.UserEdited, field)
}

// MarkUserEdited records that the user changed field by hand.
func (d *Document) MarkUserEdited(field string) {
	if !d.IsUserEdited(field) {
		d.UserEdited = append(d.UserEdited, field)
		slices.Sort(d.UserEdited)
	}
}

// UnmarkUserEdited forgets a manual change so automatic values apply again.
func (d *Document) UnmarkUserEdited(field string) {
	d.UserEdited = slices.DeleteFunc(d.UserEdited, func(f string) bool { return f == field })
	if len(d.UserEdited) == 0 {
		d.UserEdited = nil
	}
}

// StringsFromField converts a stored multi valued index field into strings.
func StringsFromField(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	}
	return nil
}

const (
	// MaxTags is the maximum number of tags a document can carry.
	MaxTags = 64
	// MaxTagLength is the maximum length of a tag in bytes.
	MaxTagLength = 64
)

// NormalizeTag lowercases a tag, trims it and collapses inner whitespace into
// single dashes so that a tag is always one searchable keyword. It returns an
// empty string for tags without visible characters.
func NormalizeTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	tag = strings.TrimPrefix(tag, "#")
	var b strings.Builder
	dash := false
	for _, r := range tag {
		switch {
		case unicode.IsSpace(r) || r == ',':
			dash = b.Len() > 0
		case unicode.IsControl(r) || r == '"':
		default:
			if dash {
				b.WriteByte('-')
				dash = false
			}
			b.WriteRune(r)
		}
	}
	out := b.String()
	for len(out) > MaxTagLength {
		_, size := utf8.DecodeLastRuneInString(out)
		out = out[:len(out)-size]
	}
	return out
}

// NormalizeTags normalizes, deduplicates and sorts tags. A nil or empty input
// returns nil.
func NormalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if t := NormalizeTag(tag); t != "" {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > MaxTags {
		out = out[:MaxTags]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// TagsFromField converts a stored index field value into tags. Bleve returns
// a string for single valued fields and a slice for multi valued ones.
func TagsFromField(v any) []string {
	return NormalizeTags(StringsFromField(v))
}
