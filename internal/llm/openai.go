package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIProvider talks to OpenAI's Responses API directly over net/http. It
// mirrors the Anthropic provider: no SDK dependency, just the small request and
// response shape Samadhan needs.
type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// OpenAIConfig configures the provider.
type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string // defaults to https://api.openai.com
	Timeout time.Duration
}

// NewOpenAI builds an OpenAI-backed provider.
func NewOpenAI(cfg OpenAIConfig) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &OpenAIProvider{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		http:    &http.Client{Timeout: cfg.Timeout},
	}
}

func (p *OpenAIProvider) Name() string { return "openai:" + p.model }

type openAIRequest struct {
	Model           string                 `json:"model"`
	Instructions    string                 `json:"instructions,omitempty"`
	Input           string                 `json:"input"`
	MaxOutputTokens int                    `json:"max_output_tokens,omitempty"`
	Temperature     float64                `json:"temperature,omitempty"`
	Text            map[string]interface{} `json:"text,omitempty"`
}

type openAIResponse struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Complete implements Provider.
func (p *OpenAIProvider) Complete(ctx context.Context, req Request) (Response, error) {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 1024
	}

	body, err := json.Marshal(openAIRequest{
		Model:           p.model,
		Instructions:    req.System,
		Input:           req.Prompt,
		MaxOutputTokens: maxTok,
		Temperature:     req.Temperature,
		Text:            map[string]interface{}{"format": map[string]string{"type": "text"}},
	})
	if err != nil {
		return Response{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("authorization", "Bearer "+p.apiKey)

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed openAIResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Response{}, fmt.Errorf("openai: decode response (status %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return Response{}, fmt.Errorf("openai: api error: %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("openai: unexpected status %d: %s", resp.StatusCode, string(raw))
	}

	text := parsed.OutputText
	if text == "" {
		text = collectOpenAIText(parsed)
	}
	if strings.TrimSpace(text) == "" {
		return Response{}, ErrEmptyResponse
	}

	tokens := parsed.Usage.TotalTokens
	if tokens == 0 {
		tokens = parsed.Usage.InputTokens + parsed.Usage.OutputTokens
	}
	return Response{
		Text:     text,
		Provider: p.Name(),
		Tokens:   tokens,
	}, nil
}

func collectOpenAIText(resp openAIResponse) string {
	var sb strings.Builder
	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		for _, c := range out.Content {
			if c.Type == "output_text" || c.Type == "text" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}
