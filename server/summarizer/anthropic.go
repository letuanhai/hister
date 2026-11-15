// SPDX-License-Identifier: AGPL-3.0-or-later

package summarizer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/asciimoo/hister/config"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// anthropicSummarizer calls Claude through the Anthropic Messages API.
type anthropicSummarizer struct {
	cfg    config.Summarizer
	client anthropic.Client
}

func newAnthropic(cfg config.Summarizer) *anthropicSummarizer {
	opts := []option.RequestOption{option.WithRequestTimeout(timeout(cfg))}
	// Without a configured key the SDK resolves ANTHROPIC_API_KEY or another
	// credential source from the environment.
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	for k, v := range cfg.Headers {
		opts = append(opts, option.WithHeader(k, v))
	}
	return &anthropicSummarizer{cfg: cfg, client: anthropic.NewClient(opts...)}
}

func (s *anthropicSummarizer) Summarize(ctx context.Context, in Input) (string, error) {
	resp, err := s.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     s.cfg.Model,
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: s.cfg.Prompt}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(userMessage(in, s.cfg.MaxInputChars))),
		},
		// A short summary does not need deep reasoning.
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortLow},
		// Retry a declined request on a fallback model chosen by the API.
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return "", fmt.Errorf("summary request failed with status %d", apiErr.StatusCode)
		}
		return "", fmt.Errorf("summary request failed: %w", err)
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return "", errors.New("the model declined to summarize this document")
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	return cleanSummary(text.String())
}
