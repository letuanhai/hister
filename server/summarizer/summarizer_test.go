package summarizer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asciimoo/hister/config"
)

func TestOpenAISummarizer(t *testing.T) {
	var got chatRequest
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"<think>hmm</think>\n A short summary. "}}]}`))
	}))
	defer srv.Close()
	s, err := New(config.Summarizer{Enable: true, Provider: "openai", Endpoint: srv.URL, APIKey: "k", MaxInputChars: 600})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := s.Summarize(context.Background(), Input{URL: "https://e.com", Title: "T", Text: strings.Repeat("x", 1000)})
	if err != nil {
		t.Fatal(err)
	}
	if summary != "A short summary." {
		t.Errorf("summary = %q", summary)
	}
	if auth != "Bearer k" || got.Model != "qwen3:8b" || len(got.Messages) != 2 || got.Messages[0].Content != DefaultPrompt {
		t.Errorf("request = %+v auth %q", got, auth)
	}
	if !strings.Contains(got.Messages[1].Content, "[document shortened]") || !strings.Contains(got.Messages[1].Content, strings.Repeat("x", 600)+"\n[") || strings.Contains(got.Messages[1].Content, strings.Repeat("x", 601)) {
		t.Error("long input was not shortened")
	}
}

func TestOpenAISummarizerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
	}))
	defer srv.Close()
	s, _ := New(config.Summarizer{Enable: true, Endpoint: srv.URL})
	if _, err := s.Summarize(context.Background(), Input{Text: "t"}); err == nil || !strings.Contains(err.Error(), "slow down") {
		t.Errorf("error = %v", err)
	}
	if _, err := New(config.Summarizer{}); err != ErrDisabled {
		t.Errorf("disabled error = %v", err)
	}
	if _, err := New(config.Summarizer{Enable: true, Provider: "other"}); err == nil {
		t.Error("unknown provider accepted")
	}
}

func TestAnthropicSummarizer(t *testing.T) {
	var body map[string]any
	var beta, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		beta = r.Header.Get("anthropic-beta")
		key = r.Header.Get("x-api-key") + r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5",
			"content":[{"type":"text","text":"Claude summary."}],"stop_reason":"end_turn",
			"usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer srv.Close()
	s, err := New(config.Summarizer{Enable: true, Provider: "anthropic", Endpoint: srv.URL, APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := s.Summarize(context.Background(), Input{Title: "T", Text: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	if summary != "Claude summary." {
		t.Errorf("summary = %q", summary)
	}
	if body["model"] != "claude-opus-5-5" || body["fallbacks"] != "default" || !strings.Contains(key, "sk-test") ||
		!strings.Contains(beta, "server-side-fallback-2026-07-01") {
		t.Errorf("request key %q, beta %q, model %v, fallbacks %v", key, beta, body["model"], body["fallbacks"])
	}
	if oc, _ := body["output_config"].(map[string]any); oc["effort"] != "low" {
		t.Errorf("output_config = %v", body["output_config"])
	}
}

func TestAnthropicSummarizerRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5",
			"content":[],"stop_reason":"refusal","usage":{"input_tokens":1,"output_tokens":0}}`))
	}))
	defer srv.Close()
	s, _ := New(config.Summarizer{Enable: true, Provider: "anthropic", Endpoint: srv.URL, APIKey: "k"})
	if _, err := s.Summarize(context.Background(), Input{Text: "x"}); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Errorf("refusal error = %v", err)
	}
}
