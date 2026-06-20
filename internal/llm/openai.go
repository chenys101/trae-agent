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

type OpenAI struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

func NewOpenAI(apiKey, baseURL, defaultModel string) *OpenAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAI{apiKey: apiKey, baseURL: baseURL, defaultModel: defaultModel}
}

func (o *OpenAI) Name() string { return "openai" }

type openaiRequest struct {
	Model         string             `json:"model"`
	Messages      []openaiMsg        `json:"messages"`
	MaxTokens     int                `json:"max_tokens,omitempty"`
	Temperature   float64            `json:"temperature,omitempty"`
	Stream        bool               `json:"stream"`
	StreamOptions *openaiStreamOpts  `json:"stream_options,omitempty"`
}

type openaiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

func (o *OpenAI) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	model := req.Model
	if model == "" {
		model = o.defaultModel
	}

	msgs := make([]openaiMsg, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, openaiMsg{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, openaiMsg{Role: string(m.Role), Content: m.Content})
	}

	bodyReq := openaiRequest{
		Model:         model,
		Messages:      msgs,
		Temperature:   req.Temperature,
		Stream:        true,
		StreamOptions: &openaiStreamOpts{IncludeUsage: true},
	}
	if req.MaxTokens > 0 {
		bodyReq.MaxTokens = req.MaxTokens
	}

	body, err := json.Marshal(bodyReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, errBody)
	}

	ch := make(chan StreamEvent, 16)
	go o.pumpSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (o *OpenAI) pumpSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var inputTokens, outputTokens int
	var stopReason string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			select {
			case ch <- Error{Err: ctx.Err()}:
			default:
			}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			select {
			case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
			case <-ctx.Done():
			}
			return
		}

		var evt struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}

		if evt.Usage != nil {
			inputTokens = evt.Usage.PromptTokens
			outputTokens = evt.Usage.CompletionTokens
		}

		for _, choice := range evt.Choices {
			if choice.Delta.Content != "" {
				select {
				case ch <- TextDelta{Content: choice.Delta.Content}:
				case <-ctx.Done():
					return
				}
			}
			if choice.FinishReason != "" {
				stopReason = choice.FinishReason
			}
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case ch <- Error{Err: fmt.Errorf("read sse: %w", err)}:
		case <-ctx.Done():
		}
		return
	}

	// 流结束但没收到 [DONE]，也发 Done（幂等保护）
	select {
	case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
	case <-ctx.Done():
	}
}
