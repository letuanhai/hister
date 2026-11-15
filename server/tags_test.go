package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/testutil"
)

func TestTagsAndEditableDocumentData(t *testing.T) {
	cfg := testutil.InitModel(t)
	idx := newServerTestIndexer(t, cfg)
	handler := registerEndpoints(cfg, idx)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "hister://"}
	post := func(path, body string, want int) {
		t.Helper()
		r := testutil.ServeHTTP(t, handler, http.MethodPost, path, strings.NewReader(body), headers)
		if r.Code != want {
			t.Fatalf("POST %s: %d %s", path, r.Code, r.Body)
		}
	}
	const pageA = "https://example.com/a"
	const pageB = "https://example.com/b"
	post("/api/add", fmt.Sprintf(`{"url":%q,"title":"Page A","text":"alpha content","tags":["Go Lang","#reading"],"user_edited":["title"]}`, pageA), http.StatusCreated)
	post("/api/add", fmt.Sprintf(`{"url":%q,"title":"Page B","text":"beta content","tags":["golang"]}`, pageB), http.StatusCreated)

	doc := idx.GetByURLAndUser(pageA, 0)
	if !reflect.DeepEqual(doc.Tags, []string{"go-lang", "reading"}) || doc.UserEdited != nil {
		t.Fatalf("submitted document = tags %v, user edited %v", doc.Tags, doc.UserEdited)
	}

	update := func(query, changes string) {
		t.Helper()
		post("/api/update", fmt.Sprintf(`{"query":%q,"changes":%s}`, query, changes), http.StatusOK)
	}
	update(`url:"`+pageA+`"`, `{"title":"My title","summary":"Hand written summary","notes":"remember this","add_tags":["Work"],"remove_tags":["reading"]}`)

	// A resubmission keeps manual data and merges submitted tags.
	post("/api/add", fmt.Sprintf(`{"url":%q,"title":"Page A v2","text":"alpha content","summary":"auto summary","tags":["new"]}`, pageA), http.StatusCreated)
	doc = idx.GetByURLAndUser(pageA, 0)
	if doc.Title != "My title" || doc.Summary != "Hand written summary" || doc.SummarySource != document.SummarySourceUser ||
		doc.Notes != "remember this" || !reflect.DeepEqual(doc.Tags, []string{"go-lang", "new", "work"}) {
		t.Fatalf("document after resubmission = %+v", doc)
	}

	search := func(q string) int {
		t.Helper()
		r := testutil.ServeHTTP(t, handler, http.MethodGet, "/api/facets?q="+url.QueryEscape(q), nil, headers)
		var facets struct {
			Terms map[string]struct {
				Terms []struct{ Term string }
			}
		}
		if err := json.Unmarshal(r.Body.Bytes(), &facets); err != nil {
			t.Fatalf("facets %q: %v %s", q, err, r.Body)
		}
		n := 0
		for range facets.Terms["domains"].Terms {
			n++
		}
		return n
	}
	for q, want := range map[string]int{"tag:Work": 1, "tag:golang": 1, "tag:nothing": 0, "tags:go*": 1, "remember": 1, "summary:hand": 1} {
		if got := search(q); got != want {
			t.Errorf("search %q matched %d domains, want %d", q, got, want)
		}
	}

	r := testutil.ServeHTTP(t, handler, http.MethodGet, "/api/tags?prefix=go", nil, headers)
	var tags []tagSuggestion
	if err := json.Unmarshal(r.Body.Bytes(), &tags); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(tags))
	for _, tag := range tags {
		got = append(got, tag.Tag)
	}
	if !reflect.DeepEqual(got, []string{"go-lang", "golang"}) && !reflect.DeepEqual(got, []string{"golang", "go-lang"}) {
		t.Errorf("tag suggestions = %v", got)
	}

	// Resetting returns to automatic values on the next submission.
	update(`url:"`+pageA+`"`, `{"reset_title":true,"summary":"","tags":[]}`)
	post("/api/add", fmt.Sprintf(`{"url":%q,"title":"Page A v3","text":"alpha content","summary":"auto summary"}`, pageA), http.StatusCreated)
	doc = idx.GetByURLAndUser(pageA, 0)
	if doc.Title != "Page A v3" || doc.Summary != "auto summary" || doc.SummarySource != document.SummarySourceAuto ||
		doc.Tags != nil || doc.UserEdited != nil {
		t.Fatalf("document after reset = %+v", doc)
	}
}

func TestDocumentEditEndpoint(t *testing.T) {
	cfg := testutil.InitModel(t)
	idx := newServerTestIndexer(t, cfg)
	handler := registerEndpoints(cfg, idx)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "hister://"}
	const page = `https://example.com/q?a="b"`
	r := testutil.ServeHTTP(t, handler, http.MethodPost, "/api/add",
		strings.NewReader(fmt.Sprintf(`{"url":%q,"title":"Quoted","text":"quoted url content"}`, page)), headers)
	if r.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", r.Code, r.Body)
	}
	body := fmt.Sprintf(`{"url":%q,"changes":{"tags":["One","two"],"notes":"n","summary":"s","user_id":5}}`, page)
	r = testutil.ServeHTTP(t, handler, http.MethodPost, "/api/document/edit", strings.NewReader(body), headers)
	if r.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", r.Code, r.Body)
	}
	var got editableDocument
	if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Tags, []string{"one", "two"}) || got.Notes != "n" || got.Summary != "s" ||
		!reflect.DeepEqual(got.UserEdited, []string{"summary"}) {
		t.Fatalf("edited document = %+v", got)
	}
	if idx.GetByURLAndUser(page, 0) == nil {
		t.Fatal("edit must not change the owner")
	}
	r = testutil.ServeHTTP(t, handler, http.MethodPost, "/api/document/edit",
		strings.NewReader(`{"url":"https://example.com/missing","changes":{"notes":"x"}}`), headers)
	if r.Code != http.StatusNotFound {
		t.Errorf("missing document: %d", r.Code)
	}
	r = testutil.ServeHTTP(t, handler, http.MethodPost, "/api/document/edit",
		strings.NewReader(fmt.Sprintf(`{"url":%q,"changes":{}}`, page)), headers)
	if r.Code != http.StatusBadRequest {
		t.Errorf("empty changes: %d", r.Code)
	}
}
