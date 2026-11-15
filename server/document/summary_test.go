package document

import (
	"strings"
	"testing"
)

func TestAutoSummary(t *testing.T) {
	long := strings.Repeat("word ", 200)
	for _, tc := range []struct {
		name, title, description, text, want string
	}{
		{"description", "T", "A useful description of the page content.", "Body text here. More.", "A useful description of the page content."},
		{"short description falls back to text", "T", "Short", "Menu\nThis is the first real paragraph of the article. It has sentences.", "This is the first real paragraph of the article. It has sentences."},
		{"description equal to title", "The same title of this page here", "The same title of this page here", "", ""},
		{"no prose", "T", "", "Home\nAbout\nContact", ""},
		{"sentence cut", "T", "", "First sentence is long enough to count here. " + long, "First sentence is long enough to count here."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AutoSummary(tc.title, tc.description, tc.text); got != tc.want {
				t.Errorf("AutoSummary() = %q, want %q", got, tc.want)
			}
		})
	}
	if got := AutoSummary("", long, ""); len(got) > MaxAutoSummaryLength+len("…") || !strings.HasSuffix(got, "…") {
		t.Errorf("long description not truncated: %d %q", len(got), got[len(got)-10:])
	}
}

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{" Go Lang ", "#go", "go", "", "a,b", strings.Repeat("x", 100)})
	want := []string{"a-b", "go", "go-lang", strings.Repeat("x", MaxTagLength)}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("NormalizeTags() = %v, want %v", got, want)
	}
}
