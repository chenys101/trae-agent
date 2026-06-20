package llm

import (
	"context"
	"fmt"
)

type Anthropic struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

func NewAnthropic(apiKey, baseURL, defaultModel string) *Anthropic {
	return &Anthropic{apiKey: apiKey, baseURL: baseURL, defaultModel: defaultModel}
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("anthropic.Stream not implemented")
}
