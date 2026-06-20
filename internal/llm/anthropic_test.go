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

func TestAnthropic_Stream_toolCall(t *testing.T) {
	sseBody := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"read"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"file_path\":\"/tmp/a\"}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":10}}`,
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

	a := NewAnthropic("k", srv.URL, "m")
	ch, err := a.Stream(context.Background(), Request{
		Model:    "m",
		Messages: []Message{{Role: RoleUser, Content: "read /tmp/a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var toolCalls []ToolCall
	for e := range ch {
		if tc, ok := e.(ToolCallDelta); ok {
			toolCalls = append(toolCalls, ToolCall{
				ID:   tc.ID,
				Name: tc.Name,
				Args: tc.ArgsDelta,
			})
		}
	}
	if len(toolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(toolCalls))
	}
	if toolCalls[0].Name != "read" {
		t.Errorf("name = %q, want read", toolCalls[0].Name)
	}
	if toolCalls[0].ID != "tool_1" {
		t.Errorf("id = %q, want tool_1", toolCalls[0].ID)
	}
	if !strings.Contains(toolCalls[0].Args, "/tmp/a") {
		t.Errorf("args = %q, want /tmp/a", toolCalls[0].Args)
	}
}
