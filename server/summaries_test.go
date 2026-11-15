package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/summarizer"
	"github.com/asciimoo/hister/server/testutil"
)

type fakeSummarizer struct{ calls *atomic.Int32 }

func (f fakeSummarizer) Summarize(_ context.Context, in summarizer.Input) (string, error) {
	f.calls.Add(1)
	return "LLM: " + in.Title, nil
}

func TestLLMSummaries(t *testing.T) {
	var calls atomic.Int32
	previous := newSummarizer
	newSummarizer = func(cfg config.Summarizer) (summarizer.Summarizer, error) {
		if !cfg.Enable {
			return nil, summarizer.ErrDisabled
		}
		return fakeSummarizer{&calls}, nil
	}
	t.Cleanup(func() { newSummarizer = previous })

	cfg := testutil.InitModel(t)
	cfg.Summarizer.Enable = true
	cfg.Summarizer.Auto = true
	idx := newServerTestIndexer(t, cfg)
	handler := registerEndpoints(cfg, idx)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "hister://"}
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		return testutil.ServeHTTP(t, handler, http.MethodPost, path, strings.NewReader(body), headers)
	}
	const page = "https://example.com/llm"
	if r := post("/api/add", fmt.Sprintf(`{"url":%q,"title":"Auto","text":"Some page text that is long enough."}`, page)); r.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", r.Code, r.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		d := idx.GetByURLAndUser(page, 0)
		if d != nil && d.SummarySource == document.SummarySourceLLM {
			if d.Summary != "LLM: Auto" {
				t.Fatalf("background summary = %q", d.Summary)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background summary was not stored: %+v", d)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// A resubmission keeps the LLM summary over a new automatic one and does not requeue.
	before := calls.Load()
	post("/api/add", fmt.Sprintf(`{"url":%q,"title":"Auto","text":"Some page text that is long enough.","summary":"meta description"}`, page))
	time.Sleep(100 * time.Millisecond)
	if d := idx.GetByURLAndUser(page, 0); d.Summary != "LLM: Auto" || calls.Load() != before {
		t.Fatalf("resubmission summary = %q, calls %d -> %d", d.Summary, before, calls.Load())
	}

	// A manual summary is replaced only when the user asks for an LLM summary.
	post("/api/document/edit", fmt.Sprintf(`{"url":%q,"changes":{"summary":"mine","title":"Renamed"}}`, page))
	r := post("/api/document/summarize", fmt.Sprintf(`{"url":%q}`, page))
	if r.Code != http.StatusOK {
		t.Fatalf("summarize: %d %s", r.Code, r.Body)
	}
	var got editableDocument
	if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Summary != "LLM: Renamed" || got.SummarySource != document.SummarySourceLLM || len(got.UserEdited) != 1 || got.UserEdited[0] != "title" {
		t.Fatalf("summarized document = %+v", got)
	}

	cfg.Summarizer.Enable = false
	if r := post("/api/document/summarize", fmt.Sprintf(`{"url":%q}`, page)); r.Code != http.StatusConflict {
		t.Errorf("disabled summarize: %d", r.Code)
	}
}
