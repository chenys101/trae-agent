package llm

import (
	"context"
	"errors"
	"testing"
)

type fakeProvider struct {
	events []StreamEvent
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, len(f.events))
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

func TestStreamEvent_implements(t *testing.T) {
	var e StreamEvent
	e = TextDelta{Content: "hi"}
	e = Done{Usage: Usage{InputTokens: 1, OutputTokens: 2}}
	e = Error{Err: errors.New("boom")}
	_ = e
}

func TestFakeProvider_drainsEvents(t *testing.T) {
	fp := &fakeProvider{events: []StreamEvent{
		TextDelta{Content: "a"},
		TextDelta{Content: "b"},
		Done{Usage: Usage{InputTokens: 10, OutputTokens: 5}},
	}}
	ch, err := fp.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	var got []StreamEvent
	for e := range ch {
		got = append(got, e)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
}

func TestNewProvider_unknown(t *testing.T) {
	_, err := NewProvider("unknown", "k", "", "m")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestNewProvider_anthropic(t *testing.T) {
	p, err := NewProvider("anthropic", "k", "", "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("name = %q, want anthropic", p.Name())
	}
}

func TestNewProvider_openai(t *testing.T) {
	p, err := NewProvider("openai", "k", "", "m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("name = %q, want openai", p.Name())
	}
}
