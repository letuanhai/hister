// SPDX-License-Identifier: AGPL-3.0-or-later

package types

// DocumentChanges describes mutable indexed document attributes. Pointer
// fields distinguish an omitted change from setting a field to its zero value.
type DocumentChanges struct {
	UserID *uint   `json:"user_id,omitempty"`
	Label  *string `json:"label,omitempty"`
	Title  *string `json:"title,omitempty"`
	// Language pins a code or unknown. The value auto clears the pin and reruns detection.
	Language *string `json:"language,omitempty"`
	// ResetTitle forgets a manual title so the next page submission restores
	// the page's own title. It is ignored when Title is set.
	ResetTitle bool `json:"reset_title,omitempty"`
	// Tags replaces every tag of the selected documents.
	Tags *[]string `json:"tags,omitempty"`
	// AddTags adds tags to the selected documents. It applies after Tags.
	AddTags []string `json:"add_tags,omitempty"`
	// RemoveTags removes tags from the selected documents. It applies last.
	RemoveTags []string `json:"remove_tags,omitempty"`
	// Summary replaces the summary. An empty value returns to the automatic summary.
	Summary *string `json:"summary,omitempty"`
	// Notes replaces the user notes.
	Notes *string `json:"notes,omitempty"`
}

// Empty reports whether no document attribute changes were requested.
func (c DocumentChanges) Empty() bool {
	return c.UserID == nil && c.Label == nil && c.Title == nil && !c.ResetTitle && c.Language == nil &&
		c.Tags == nil && len(c.AddTags) == 0 && len(c.RemoveTags) == 0 && c.Summary == nil && c.Notes == nil
}

// UpdateDocumentsRequest selects documents and describes the changes to apply.
type UpdateDocumentsRequest struct {
	Query   string          `json:"query"`
	Changes DocumentChanges `json:"changes"`
	DryRun  bool            `json:"dry_run,omitempty"`
}

// DocumentOwnershipChange records an applied document identity change.
// OwnershipChanges is internal server state and is not included in responses.
type DocumentOwnershipChange struct {
	URL        string
	FromUserID uint
	ToUserID   uint
}

// UpdateDocumentsResult summarizes a document update operation.
type UpdateDocumentsResult struct {
	Matched          int                       `json:"matched"`
	Updated          int                       `json:"updated"`
	Unchanged        int                       `json:"unchanged"`
	Conflicts        int                       `json:"conflicts"`
	OwnershipChanges []DocumentOwnershipChange `json:"-"`
}
