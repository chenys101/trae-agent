package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCommand_streamsToStdout(t *testing.T) {
	// mock anthropic SSE
	sseBody := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hi there"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
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

	// 写用户配置指向 mock server
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgPath := filepath.Join(tmp, ".trae", "config.yaml")
	os.MkdirAll(filepath.Dir(cfgPath), 0o755)
	os.WriteFile(cfgPath, []byte(`
default_provider: anthropic
model_providers:
  anthropic:
    api_key: test-key
    base_url: `+srv.URL+`
    default_model: claude-3-5-sonnet-20241022
`), 0o644)

	var buf bytes.Buffer
	cmd := NewRunCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"hello"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Hi there") {
		t.Errorf("output missing 'Hi there', got: %s", buf.String())
	}
}
