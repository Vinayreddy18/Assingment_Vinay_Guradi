package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicProvider talks to the Anthropic Messages API directly over
// net/http. No SDK dependency is pulled in — the request/response shape is
// small and stable, and keeping the binary dependency-free matters for a
// service meant to run at high volume in constrained environments.
type AnthropicProvider struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// AnthropicConfig configures the provider.
type AnthropicConfig struct {
	APIKey  string
	Model   string
	BaseURL string // defaults to https://api.anthropic.com
	Timeout time.Duration
}

// NewAnthropic builds an Anthropic-backed provider.
func NewAnthropic(cfg AnthropicConfig) *AnthropicProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &AnthropicProvider{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: cfg.BaseURL,
		http:    &http.Client{Timeout: cfg.Timeout},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic:" + p.model }

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Temperature float64            `json:"temperature"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete implements Provider.
func (p *AnthropicProvider) Complete(ctx context.Context, req Request) (Response, error) {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 1024
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       p.model,
		MaxTokens:   maxTok,
		System:      req.System,
		Temperature: req.Temperature,
		Messages:    []anthropicMessage{{Role: "user", Content: req.Prompt}},
	})
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Response{}, fmt.Errorf("anthropic: decode response (status %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return Response{}, fmt.Errorf("anthropic: api error: %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode, string(raw))
	}

	var sb bytes.Buffer
	for _, c := range parsed.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if sb.Len() == 0 {
		return Response{}, ErrEmptyResponse
	}
	return Response{
		Text:     sb.String(),
		Provider: p.Name(),
		Tokens:   parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
	}, nil
}
