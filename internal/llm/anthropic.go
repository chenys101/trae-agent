package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// httpClient 为 llm 包共享的 HTTP 客户端。
// 流式场景需要长连接，故不设置 Client.Timeout；转而在 Transport 层
// 设置连接/握手/响应头超时，避免请求卡死在建立连接阶段。
var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

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
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []anthropicMsg  `json:"messages"`
	System      string          `json:"system,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
	Tools       []anthropicTool `json:"tools,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   any             `json:"content,omitempty"`
}

type anthropicMsg struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

// toolCallAccum 累积流式 tool_call 增量（包级共享，openai.go 也用）。
type toolCallAccum struct {
	ID   string
	Name string
	Args strings.Builder
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

	var tools []anthropicTool
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Schema,
		})
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Messages:    toAnthropicMsgs(req.Messages),
		System:      req.System,
		Temperature: req.Temperature,
		Stream:      true,
		Tools:       tools,
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

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		// 限制读取 4KB，避免错误响应体过大或为二进制时污染日志/错误信息。
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	ch := make(chan StreamEvent, 16)
	go a.pumpSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (a *Anthropic) pumpSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	// 上限 10MB，避免大 tool_call 参数（如长文件内容）被截断导致 JSON 不完整。
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var inputTokens, outputTokens int
	var stopReason string
	toolAccums := map[int]*toolCallAccum{}

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
		if data == "" || data == "[DONE]" {
			continue
		}

		var evt struct {
			Type         string          `json:"type"`
			Index        int             `json:"index"`
			Delta        json.RawMessage `json:"delta"`
			ContentBlock json.RawMessage `json:"content_block"`
			Usage        struct {
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
		case "content_block_start":
			var blk struct {
				Index int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &blk); err != nil {
				select {
				case ch <- Error{Err: fmt.Errorf("anthropic: parse content_block_start: %w", err)}:
				case <-ctx.Done():
					return
				}
				continue
			}
			if blk.ContentBlock.Type == "tool_use" {
				toolAccums[blk.Index] = &toolCallAccum{
					ID:   blk.ContentBlock.ID,
					Name: blk.ContentBlock.Name,
				}
			}
		case "content_block_delta":
			var d struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			}
			if err := json.Unmarshal(evt.Delta, &d); err != nil {
				select {
				case ch <- Error{Err: fmt.Errorf("anthropic: parse content_block_delta: %w", err)}:
				case <-ctx.Done():
					return
				}
				continue
			}
			if d.Type == "text_delta" {
				select {
				case ch <- TextDelta{Content: d.Text}:
				case <-ctx.Done():
					return
				}
			} else if d.Type == "input_json_delta" {
				if acc, ok := toolAccums[evt.Index]; ok {
					acc.Args.WriteString(d.PartialJSON)
				}
			}
		case "message_delta":
			if evt.Usage.OutputTokens > 0 {
				outputTokens = evt.Usage.OutputTokens
			}
			var d struct {
				StopReason string `json:"stop_reason"`
			}
			if err := json.Unmarshal(evt.Delta, &d); err != nil {
				select {
				case ch <- Error{Err: fmt.Errorf("anthropic: parse message_delta: %w", err)}:
				case <-ctx.Done():
					return
				}
				continue
			}
			if d.StopReason != "" {
				stopReason = d.StopReason
			}
		case "message_stop":
			// 按 index 有序 flush，避免混合 content block（text 在前、tool_use 在后）
			// 时按 map 大小遍历漏掉非 0 起始的 tool_use
			flushAnthropicToolCalls(ctx, ch, toolAccums)
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
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				select {
				case ch <- Error{Err: fmt.Errorf("anthropic: parse error event: %w", err)}:
				case <-ctx.Done():
					return
				}
				continue
			}
			select {
			case ch <- Error{Err: fmt.Errorf("anthropic stream error: %s", e.Error.Message)}:
			case <-ctx.Done():
			}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case ch <- Error{Err: fmt.Errorf("read sse: %w", err)}:
		case <-ctx.Done():
		}
		return
	}
	// 流结束但未收到 message_stop（连接被截断/服务端 bug），兜底发 Done
	// 避免消费者无法区分正常结束与异常中断
	flushAnthropicToolCalls(ctx, ch, toolAccums)
	select {
	case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
	case <-ctx.Done():
	}
}

// flushAnthropicToolCalls 按 index 升序 flush 工具调用，保证顺序稳定。
func flushAnthropicToolCalls(ctx context.Context, ch chan<- StreamEvent, toolAccums map[int]*toolCallAccum) {
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

func toAnthropicMsgs(msgs []Message) []anthropicMsg {
	out := make([]anthropicMsg, len(msgs))
	for i, m := range msgs {
		var blocks []anthropicContentBlock
		role := string(m.Role)
		if m.ToolCallID != "" {
			role = "user"
			blocks = []anthropicContentBlock{{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}}
		} else if len(m.ToolCalls) > 0 {
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(tc.Args),
				})
			}
		} else {
			blocks = []anthropicContentBlock{{Type: "text", Text: m.Content}}
		}
		out[i] = anthropicMsg{Role: role, Content: blocks}
	}
	return out
}
