package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `{"a":1}`, `{"a":1}`},
		{"fenced", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"bare fence", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"surrounding prose", `Sure! Here is the result: {"a":1} hope that helps`, `{"a":1}`},
		{"nested", `{"a":{"b":2},"c":3}`, `{"a":{"b":2},"c":3}`},
		{"brace in string", `{"msg":"a } b { c"}`, `{"msg":"a } b { c"}`},
		{"escaped quote in string", `{"msg":"she said \"hi\" }"}`, `{"msg":"she said \"hi\" }"}`},
		{"none", `no json here`, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractJSON(c.in); got != c.want {
				t.Errorf("extractJSON(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestCompleteJSON_Mock verifies the generic JSON completion path end to end
// against the deterministic provider.
func TestCompleteJSON_Mock(t *testing.T) {
	type assess struct {
		ClaimStrength float64 `json:"claim_strength"`
	}
	facts := map[string]any{
		"category":     "cheque_bounce",
		"claim_amount": int64(250000),
		"narrative":    "cheque dishonoured",
	}
	out, err := CompleteJSON[assess](context.Background(), NewMock(), CaseAssessmentRequest(facts))
	if err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if out.ClaimStrength <= 0 || out.ClaimStrength > 1 {
		t.Errorf("claim_strength out of range: %v", out.ClaimStrength)
	}
}

func TestOpenAIProvider_Complete(t *testing.T) {
	var gotAuth string
	var gotReq openAIRequest
	p := NewOpenAI(OpenAIConfig{
		APIKey:  "test-key",
		Model:   "gpt-test",
		BaseURL: "https://example.test",
	})
	p.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("authorization")
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %s, want /v1/responses", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"content-type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewBufferString(`{
			"output_text":"{\"message\":\"ok\"}",
			"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}
		}`)),
		}, nil
	})}
	resp, err := p.Complete(context.Background(), Request{
		System:      "system",
		Prompt:      "prompt",
		MaxTokens:   123,
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("authorization header = %q", gotAuth)
	}
	if gotReq.Model != "gpt-test" || gotReq.Input != "prompt" || gotReq.Instructions != "system" {
		t.Errorf("unexpected request: %+v", gotReq)
	}
	if resp.Text != `{"message":"ok"}` {
		t.Errorf("text = %q", resp.Text)
	}
	if resp.Provider != "openai:gpt-test" {
		t.Errorf("provider = %q", resp.Provider)
	}
	if resp.Tokens != 5 {
		t.Errorf("tokens = %d", resp.Tokens)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
