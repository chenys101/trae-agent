package headless

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestRun_streamsTextToWriter(t *testing.T) {
	provider := &fakeProvider{
		events: []llm.StreamEvent{
			llm.TextDelta{Content: "Hello"},
			llm.TextDelta{Content: " world"},
			llm.Done{Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}
	var buf bytes.Buffer
	usage, err := Run(context.Background(), provider, llm.Request{
		Model:    "test",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, &buf)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if buf.String() != "Hello world" {
		t.Errorf("output = %q, want 'Hello world'", buf.String())
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want {10, 5}", usage)
	}
}

func TestRun_streamError(t *testing.T) {
	provider := &fakeProvider{
		events: []llm.StreamEvent{
			llm.TextDelta{Content: "partial"},
			llm.Error{Err: errors.New("stream broke")},
		},
	}
	var buf bytes.Buffer
	_, err := Run(context.Background(), provider, llm.Request{}, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if buf.String() != "partial" {
		t.Errorf("partial output = %q, want 'partial'", buf.String())
	}
}

func TestRun_providerError(t *testing.T) {
	provider := &fakeProvider{streamErr: errors.New("provider init failed")}
	var buf bytes.Buffer
	_, err := Run(context.Background(), provider, llm.Request{}, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

type fakeProvider struct {
	events    []llm.StreamEvent
	streamErr error
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	ch := make(chan llm.StreamEvent, len(f.events))
	go func() {
		defer close(ch)
		for _, e := range f.events {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch, nil
}
