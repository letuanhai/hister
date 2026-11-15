package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asciimoo/hister/server/model"
	"github.com/asciimoo/hister/server/testutil"
)

const archivePageHTML = `<html><head><title>Archived page</title></head><body>
<nav>Menu Home About</nav>
<article><h1>Archived page</h1>
<p>This is the first paragraph of an article that is long enough to be kept by reader extraction.</p>
<p>The second paragraph adds more sentences so that the main content is clearly the article text.</p>
<p>A third paragraph completes the article with a few more words about the topic.</p>
<script>alert(1)</script></article><footer>Footer links</footer></body></html>`

func TestArchiveEndpoints(t *testing.T) {
	cfg := testutil.InitModel(t)
	cfg.Archiver.Enable = true
	cfg.Archiver.KeepImages = false
	idx := newServerTestIndexer(t, cfg)
	handler := registerEndpoints(cfg, idx)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "hister://"}
	const page = "https://example.com/archive-me"
	body, _ := json.Marshal(map[string]string{"url": page, "html": archivePageHTML})
	if r := testutil.ServeHTTP(t, handler, http.MethodPost, "/api/add", strings.NewReader(string(body)), headers); r.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", r.Code, r.Body)
	}

	r := testutil.ServeHTTP(t, handler, http.MethodPost, "/api/archive", strings.NewReader(fmt.Sprintf(`{"url":%q}`, page)), headers)
	if r.Code != http.StatusCreated {
		t.Fatalf("archive: %d %s", r.Code, r.Body)
	}
	var rec model.DocumentArchive
	if err := json.Unmarshal(r.Body.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Status != model.ArchiveReady || rec.Engine != "stored" || rec.Title != "Archived page" || rec.Size == 0 {
		t.Fatalf("archive record = %+v", rec)
	}

	r = testutil.ServeHTTP(t, handler, http.MethodGet, fmt.Sprintf("/api/archive/%d/", rec.ID), nil, headers)
	if r.Code != http.StatusOK {
		t.Fatalf("archive page: %d %s", r.Code, r.Body)
	}
	if csp := r.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("archive CSP = %q", csp)
	}
	s := r.Body.String()
	if !strings.Contains(s, "first paragraph of an article") || strings.Contains(s, "<script") || strings.Contains(s, "Footer links") {
		t.Errorf("archive page:\n%s", s)
	}
	if r := testutil.ServeHTTP(t, handler, http.MethodGet, fmt.Sprintf("/api/archive/%d/../../etc/passwd", rec.ID), nil, headers); r.Code == http.StatusOK {
		t.Error("archive serves arbitrary files")
	}

	r = testutil.ServeHTTP(t, handler, http.MethodGet, "/api/archives?url="+url.QueryEscape(page), nil, headers)
	if !strings.Contains(r.Body.String(), `"total":1`) {
		t.Errorf("archive list: %s", r.Body)
	}
	r = testutil.ServeHTTP(t, handler, http.MethodGet, "/api/archives", nil, headers)
	if !strings.Contains(r.Body.String(), `"total":1`) {
		t.Errorf("all archives: %s", r.Body)
	}

	// Cookies upload, status and removal.
	cookies := ".example.com\tTRUE\t/\tTRUE\t0\tsid\tabc\n"
	if r := testutil.ServeHTTP(t, handler, http.MethodPost, "/api/archiver/cookies", strings.NewReader(cookies), headers); r.Code != http.StatusOK {
		t.Fatalf("cookies: %d %s", r.Code, r.Body)
	}
	r = testutil.ServeHTTP(t, handler, http.MethodGet, "/api/archiver", nil, headers)
	if !strings.Contains(r.Body.String(), `"has_cookies":true`) || !strings.Contains(r.Body.String(), `"enabled":true`) {
		t.Errorf("archiver status: %s", r.Body)
	}
	if r := testutil.ServeHTTP(t, handler, http.MethodPost, "/api/archiver/cookies", strings.NewReader("garbage"), headers); r.Code != http.StatusBadRequest {
		t.Errorf("invalid cookies: %d", r.Code)
	}
	testutil.ServeHTTP(t, handler, http.MethodPost, "/api/archiver/cookies", strings.NewReader(""), headers)
	if _, err := os.Stat(cookiesPath(idx, 0)); !os.IsNotExist(err) {
		t.Error("cookies file not removed")
	}

	// Deleting the document removes its archives and files.
	stored, err := model.GetArchive(rec.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stored.Dir, "index.html")); err != nil {
		t.Fatal(err)
	}
	testutil.ServeHTTP(t, handler, http.MethodPost, "/api/delete", strings.NewReader(fmt.Sprintf(`{"query":%q}`, `url:"`+page+`"`)), headers)
	if _, err := model.GetArchive(rec.ID, 0); err == nil {
		t.Error("archive record kept after document deletion")
	}
	if _, err := os.Stat(stored.Dir); !os.IsNotExist(err) {
		t.Error("archive files kept after document deletion")
	}

	cfg.Archiver.Enable = false
	if r := testutil.ServeHTTP(t, handler, http.MethodPost, "/api/archive", strings.NewReader(fmt.Sprintf(`{"url":%q}`, page)), headers); r.Code != http.StatusConflict {
		t.Errorf("disabled archive: %d", r.Code)
	}
}
