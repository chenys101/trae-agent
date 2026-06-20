package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAI_Stream_text(t *testing.T) {
	sseBody := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		``,
		`data: [DONE]`,
		``,
		``,
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	o := NewOpenAI("test-key", srv.URL, "gpt-4o")
	ch, err := o.Stream(context.Background(), Request{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var text string
	var usage Usage
	var stopReason string
	for e := range ch {
		switch ev := e.(type) {
		case TextDelta:
			text += ev.Content
		case Done:
			usage = ev.Usage
			stopReason = ev.StopReason
		case Error:
			t.Fatalf("stream error: %v", ev.Err)
		}
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want 'Hello world'", text)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want {10, 5}", usage)
	}
	if stopReason != "stop" {
		t.Errorf("stopReason = %q, want stop", stopReason)
	}
}

func TestOpenAI_Stream_apiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	o := NewOpenAI("bad-key", srv.URL, "gpt-4o")
	_, err := o.Stream(context.Background(), Request{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestOpenAI_Stream_systemPrompt(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	o := NewOpenAI("k", srv.URL, "gpt-4o")
	_, _ = o.Stream(context.Background(), Request{
		Model:    "gpt-4o",
		System:   "You are helpful.",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})

	if !strings.Contains(gotBody, `"role":"system"`) {
		t.Errorf("request body missing system message: %s", gotBody)
	}
	if !strings.Contains(gotBody, "You are helpful.") {
		t.Errorf("request body missing system content: %s", gotBody)
	}
}
