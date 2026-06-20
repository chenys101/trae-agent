package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Anthropic struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

func NewAnthropic(apiKey, baseURL, defaultModel string) *Anthropic {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &Anthropic{apiKey: apiKey, baseURL: baseURL, defaultModel: defaultModel}
}

func (a *Anthropic) Name() string { return "anthropic" }

type anthropicRequest struct {
	Model       string         `json:"model"`
	MaxTokens   int            `json:"max_tokens"`
	Messages    []anthropicMsg `json:"messages"`
	System      string         `json:"system,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	Stream      bool           `json:"stream"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (a *Anthropic) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	model := req.Model
	if model == "" {
		model = a.defaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Messages:    toAnthropicMsgs(req.Messages),
		System:      req.System,
		Temperature: req.Temperature,
		Stream:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, errBody)
	}

	ch := make(chan StreamEvent, 16)
	go a.pumpSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (a *Anthropic) pumpSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var inputTokens, outputTokens int
	var stopReason string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- Error{Err: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var evt struct {
			Type  string          `json:"type"`
			Delta json.RawMessage `json:"delta"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Message struct {
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}

		switch evt.Type {
		case "message_start":
			inputTokens = evt.Message.Usage.InputTokens
		case "content_block_delta":
			var d struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(evt.Delta, &d); err == nil && d.Type == "text_delta" {
				select {
				case ch <- TextDelta{Content: d.Text}:
				case <-ctx.Done():
					return
				}
			}
		case "message_delta":
			if evt.Usage.OutputTokens > 0 {
				outputTokens = evt.Usage.OutputTokens
			}
			var d struct {
				StopReason string `json:"stop_reason"`
			}
			json.Unmarshal(evt.Delta, &d)
			if d.StopReason != "" {
				stopReason = d.StopReason
			}
		case "message_stop":
			select {
			case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
			case <-ctx.Done():
			}
			return
		case "error":
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			json.Unmarshal([]byte(data), &e)
			select {
			case ch <- Error{Err: fmt.Errorf("anthropic stream error: %s", e.Error.Message)}:
			case <-ctx.Done():
			}
			return
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		select {
		case ch <- Error{Err: fmt.Errorf("read sse: %w", err)}:
		case <-ctx.Done():
		}
	}
}

func toAnthropicMsgs(msgs []Message) []anthropicMsg {
	out := make([]anthropicMsg, len(msgs))
	for i, m := range msgs {
		out[i] = anthropicMsg{Role: string(m.Role), Content: m.Content}
	}
	return out
}
