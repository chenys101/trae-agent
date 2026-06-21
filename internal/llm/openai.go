package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	return &OpenAI{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), defaultModel: defaultModel}
}

func (o *OpenAI) Name() string { return "openai" }

type openaiRequest struct {
	Model         string             `json:"model"`
	Messages      []openaiMsg        `json:"messages"`
	MaxTokens     int                `json:"max_tokens,omitempty"`
	Temperature   float64            `json:"temperature,omitempty"`
	Stream        bool               `json:"stream"`
	StreamOptions *openaiStreamOpts  `json:"stream_options,omitempty"`
	Tools         []openaiTool       `json:"tools,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiToolFunc `json:"function"`
}

type openaiToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openaiMsg struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
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
		om := openaiMsg{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			oc := openaiToolCall{ID: tc.ID, Type: "function"}
			oc.Function.Name = tc.Name
			oc.Function.Arguments = tc.Args
			om.ToolCalls = append(om.ToolCalls, oc)
		}
		msgs = append(msgs, om)
	}

	var tools []openaiTool
	for _, t := range req.Tools {
		tools = append(tools, openaiTool{
			Type: "function",
			Function: openaiToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		})
	}

	bodyReq := openaiRequest{
		Model:         model,
		Messages:      msgs,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		Stream:        true,
		StreamOptions: &openaiStreamOpts{IncludeUsage: true},
		Tools:         tools,
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

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		// 限制读取 4KB，避免错误响应体过大或为二进制时污染日志/错误信息。
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	ch := make(chan StreamEvent, 16)
	go o.pumpSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (o *OpenAI) pumpSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	// 上限 10MB，避免大 tool_call 参数（如长文件内容）被截断导致 JSON 不完整。
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var inputTokens, outputTokens int
	var stopReason string
	toolAccums := map[int]*toolCallAccum{}

	flushToolCalls := func() {
		// 按 index 升序 flush，避免非连续 index 时漏掉工具调用
		if len(toolAccums) == 0 {
			return
		}
		indices := make([]int, 0, len(toolAccums))
		for i := range toolAccums {
			indices = append(indices, i)
		}
		sort.Ints(indices)
		for _, i := range indices {
			acc := toolAccums[i]
			select {
			case ch <- ToolCallDelta{ID: acc.ID, Name: acc.Name, ArgsDelta: acc.Args.String()}:
			case <-ctx.Done():
				return
			}
		}
	}

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
		field, data := parseSSELine(line)
		if field != "data" || data == "" {
			continue
		}
		if data == "[DONE]" {
			flushToolCalls()
			select {
			case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
			case <-ctx.Done():
			}
			return
		}

		var evt struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
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
			for _, tc := range choice.Delta.ToolCalls {
				acc, ok := toolAccums[tc.Index]
				if !ok {
					acc = &toolCallAccum{}
					toolAccums[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Function.Name != "" {
					acc.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.Args.WriteString(tc.Function.Arguments)
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
	flushToolCalls()
	select {
	case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
	case <-ctx.Done():
	}
}
