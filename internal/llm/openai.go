package llm

import (
	"context"
	"fmt"
)

type OpenAI struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

func NewOpenAI(apiKey, baseURL, defaultModel string) *OpenAI {
	return &OpenAI{apiKey: apiKey, baseURL: baseURL, defaultModel: defaultModel}
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("openai.Stream not implemented")
}
