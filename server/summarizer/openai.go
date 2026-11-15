// SPDX-License-Identifier: AGPL-3.0-or-later

package summarizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/asciimoo/hister/config"
)

const maxResponseBytes = 4 << 20

// openAISummarizer calls an OpenAI compatible /v1/chat/completions endpoint.
type openAISummarizer struct {
	cfg    config.Summarizer
	client *http.Client
}

func newOpenAI(cfg config.Summarizer) *openAISummarizer {
	return &openAISummarizer{cfg: cfg, client: &http.Client{Timeout: timeout(cfg)}}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *openAISummarizer) Summarize(ctx context.Context, in Input) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: s.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: s.cfg.Prompt},
			{Role: "user", Content: userMessage(in, s.cfg.MaxInputChars)},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	}
	for k, v := range s.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("summary request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", err
	}
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("summary request failed with status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("invalid summary response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", fmt.Errorf("summary request failed with status %d: %s", resp.StatusCode, parsed.Error.Message)
		}
		return "", fmt.Errorf("summary request failed with status %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("summary response has no choices")
	}
	return cleanSummary(parsed.Choices[0].Message.Content)
}
