// Package llm defines a provider-agnostic interface for the language-model
// calls Samadhan makes, plus live OpenAI/Anthropic implementations and a
// deterministic offline provider. Treating the model as an injected
// dependency keeps the analysis and negotiation engines testable and lets the
// whole system run with no network access for demos and CI.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Task tags the kind of reasoning a prompt is asking for. The real provider
// treats it as context; the offline provider uses it to route to the right
// deterministic handler.
type Task string

const (
	TaskCaseAssessment Task = "case_assessment"
	TaskNudge          Task = "nudge"
	TaskDraft          Task = "draft_settlement"
)

// Request is a single completion request.
type Request struct {
	Task        Task
	System      string
	Prompt      string
	MaxTokens   int
	Temperature float64
	// Facts is the structured context the prompt is built around. It is also
	// embedded as a JSON block inside Prompt so the offline provider can read
	// the same inputs the real model sees.
	Facts map[string]any
}

// Response is the model's reply.
type Response struct {
	Text     string
	Provider string
	Tokens   int // best-effort total tokens, 0 if unknown
}

// Provider is the seam every model implementation satisfies.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
}

// ErrEmptyResponse is returned when a provider yields no usable text.
var ErrEmptyResponse = errors.New("llm: empty response")

// CompleteJSON runs a completion and decodes the model's reply into T. It is
// tolerant of the common ways models wrap JSON (markdown fences, leading
// prose) and retries once with a stricter instruction if the first parse
// fails. This is the single choke point for "give me structured data back",
// so prompt-formatting quirks are handled in exactly one place.
func CompleteJSON[T any](ctx context.Context, p Provider, req Request) (T, error) {
	var zero T

	resp, err := p.Complete(ctx, req)
	if err != nil {
		return zero, err
	}
	if out, err := decodeLoose[T](resp.Text); err == nil {
		return out, nil
	}

	// Retry once, hardening the instruction.
	retry := req
	retry.Temperature = 0
	retry.Prompt = req.Prompt + "\n\nReturn ONLY a single valid JSON object. " +
		"No markdown, no code fences, no commentary."
	resp, err = p.Complete(ctx, retry)
	if err != nil {
		return zero, err
	}
	out, err := decodeLoose[T](resp.Text)
	if err != nil {
		return zero, fmt.Errorf("llm: could not parse JSON from %s: %w", p.Name(), err)
	}
	return out, nil
}

// decodeLoose extracts the first JSON object from s and unmarshals it.
func decodeLoose[T any](s string) (T, error) {
	var zero T
	raw := extractJSON(s)
	if raw == "" {
		return zero, ErrEmptyResponse
	}
	var out T
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return zero, err
	}
	return out, nil
}

// extractJSON pulls the substring from the first '{' to its matching '}',
// ignoring braces inside strings. It strips markdown fences first.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")

	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case c == '{' && !inStr:
			depth++
		case c == '}' && !inStr:
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
