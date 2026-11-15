// SPDX-License-Identifier: AGPL-3.0-or-later

package types

import "time"

// PreviewDocumentDetails contains the stored document properties that can be
// used to understand and construct search queries. Large content and internal
// storage fields are deliberately excluded.
type PreviewDocumentDetails struct {
	Type     string         `json:"type"`
	Language string         `json:"language"`
	Label    string         `json:"label"`
	Tags     []string       `json:"tags"`
	Visits   uint           `json:"visits"`
	UserID   uint           `json:"user_id"`
	Metadata map[string]any `json:"metadata"`
}

// DocumentPreviewResponse is the API response for a rendered document
// preview and its searchable properties.
type DocumentPreviewResponse struct {
	Title    string                 `json:"title"`
	Content  string                 `json:"content"`
	Template string                 `json:"template"`
	Added    int64                  `json:"added"`
	Updated  int64                  `json:"updated"`
	Details  PreviewDocumentDetails `json:"details"`
	// Summary, Notes and UserEdited are the user editable document data.
	Summary          string         `json:"summary,omitempty"`
	SummarySource    string         `json:"summary_source,omitempty"`
	Notes            string         `json:"notes,omitempty"`
	UserEdited       []string       `json:"user_edited,omitempty"`
	Meta             map[string]any `json:"meta,omitempty"`
	VersionID        uint           `json:"version_id,omitempty"`
	VersionCreatedAt *time.Time     `json:"version_created_at,omitempty"`
	VersionCount     int64          `json:"version_count,omitempty"`
	// VisitCount is the number of recorded visits in the visit timeline.
	VisitCount int64 `json:"visit_count,omitempty"`
}
