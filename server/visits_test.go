package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/asciimoo/hister/server/model"
	"github.com/asciimoo/hister/server/testutil"
)

func TestVisitTimelineEndpoints(t *testing.T) {
	cfg := testutil.InitModel(t)
	idx := newServerTestIndexer(t, cfg)
	handler := registerEndpoints(cfg, idx)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "hister://"}
	const pageURL = "https://example.com/visited"

	add := func(body string) {
		t.Helper()
		r := testutil.ServeHTTP(t, handler, http.MethodPost, "/api/add", strings.NewReader(body), headers)
		if r.Code != http.StatusCreated {
			t.Fatalf("add: %d %s", r.Code, r.Body)
		}
	}
	past := time.Now().Add(-48 * time.Hour).Unix()
	add(fmt.Sprintf(`{"url":%q,"title":"Visited","text":"visited page text","visited":%d}`, pageURL, past))
	add(fmt.Sprintf(`{"url":%q,"title":"Visited","text":"visited page text"}`, pageURL))
	// A resubmission while the page loads is not a new visit.
	add(fmt.Sprintf(`{"url":%q,"title":"Visited","text":"visited page text"}`, pageURL))

	batch := fmt.Sprintf(`{"ops":[{"op":"add","url":%q,"title":"Visited","text":"visited page text","visited":%d}]}`,
		pageURL, past-3600)
	if r := testutil.ServeHTTP(t, handler, http.MethodPost, "/api/batch", strings.NewReader(batch), headers); r.Code != http.StatusOK {
		t.Fatalf("batch: %d %s", r.Code, r.Body)
	}

	r := testutil.ServeHTTP(t, handler, http.MethodGet, "/api/visits?url="+url.QueryEscape(pageURL), nil, headers)
	if r.Code != http.StatusOK {
		t.Fatalf("visits: %d %s", r.Code, r.Body)
	}
	var resp visitsResponse
	if err := json.Unmarshal(r.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 3 || len(resp.Visits) != 3 {
		t.Fatalf("visits = %d/%d, want 3", resp.Total, len(resp.Visits))
	}
	if resp.Visits[1].VisitedAt.Unix() != past || resp.Visits[2].VisitedAt.Unix() != past-3600 {
		t.Errorf("imported visit times were not kept: %+v", resp.Visits)
	}

	r = testutil.ServeHTTP(t, handler, http.MethodGet, "/api/preview?url="+url.QueryEscape(pageURL), nil, headers)
	if !strings.Contains(r.Body.String(), `"visit_count":3`) {
		t.Errorf("preview lacks visit count: %s", r.Body)
	}

	r = testutil.ServeHTTP(t, handler, http.MethodPost, "/api/visits/delete",
		strings.NewReader(fmt.Sprintf(`{"id":%d}`, resp.Visits[0].ID)), headers)
	if r.Code != http.StatusNoContent {
		t.Fatalf("delete visit: %d %s", r.Code, r.Body)
	}
	if n, _ := model.CountVisits(pageURL, 0); n != 2 {
		t.Errorf("visits after delete = %d, want 2", n)
	}

	r = testutil.ServeHTTP(t, handler, http.MethodPost, "/api/delete",
		strings.NewReader(fmt.Sprintf(`{"query":%q}`, "url:\""+pageURL+"\"")), headers)
	if r.Code != http.StatusOK {
		t.Fatalf("delete document: %d %s", r.Code, r.Body)
	}
	if n, _ := model.CountVisits(pageURL, 0); n != 0 {
		t.Errorf("visits remain after document deletion: %d", n)
	}

	r = testutil.ServeHTTP(t, handler, http.MethodGet, "/api/visits?url="+url.QueryEscape(pageURL), nil, headers)
	if r.Code != http.StatusNotFound {
		t.Errorf("visits of a deleted document: %d", r.Code)
	}
}
