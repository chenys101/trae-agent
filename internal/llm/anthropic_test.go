package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropic_Stream_text(t *testing.T) {
	sseBody := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":10}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
		``,
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	a := NewAnthropic("test-key", srv.URL, "claude-3-5-sonnet-20241022")
	ch, err := a.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
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
	if stopReason != "end_turn" {
		t.Errorf("stopReason = %q, want end_turn", stopReason)
	}
}

func TestAnthropic_Stream_apiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()

	a := NewAnthropic("bad-key", srv.URL, "claude-3-5-sonnet-20241022")
	_, err := a.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestAnthropic_Stream_contextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	a := NewAnthropic("k", srv.URL, "m")
	ch, err := a.Stream(ctx, Request{Model: "m", Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	for range ch {
	}
}
