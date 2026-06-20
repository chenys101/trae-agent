package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetry_successOnSecondAttempt(t *testing.T) {
	var calls int32
	inner := &fakeFailingProvider{
		failCount: 1,
		calls:     &calls,
		events:    []StreamEvent{TextDelta{Content: "ok"}, Done{}},
	}
	p := NewRetryable(inner, 3, time.Millisecond)
	ch, err := p.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	var got []StreamEvent
	for e := range ch {
		got = append(got, e)
	}
	if len(got) != 2 {
		t.Errorf("got %d events, want 2", len(got))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestRetry_exhausted(t *testing.T) {
	var calls int32
	inner := &fakeFailingProvider{
		failCount: 99,
		calls:     &calls,
		events:    []StreamEvent{Done{}},
	}
	p := NewRetryable(inner, 3, time.Millisecond)
	_, err := p.Stream(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetry_nonRetryableError(t *testing.T) {
	var calls int32
	inner := &fakeFailingProvider{
		failCount:   99,
		calls:       &calls,
		events:      []StreamEvent{Done{}},
		errToReturn: errors.New("400 Bad Request"),
	}
	p := NewRetryable(inner, 3, time.Millisecond)
	_, err := p.Stream(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (non-retryable)", calls)
	}
}

type fakeFailingProvider struct {
	failCount   int32
	calls       *int32
	events      []StreamEvent
	errToReturn error
}

func (f *fakeFailingProvider) Name() string { return "fake-failing" }
func (f *fakeFailingProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	n := atomic.AddInt32(f.calls, 1)
	if int32(n) <= f.failCount {
		if f.errToReturn != nil {
			return nil, f.errToReturn
		}
		return nil, errors.New("503 Service Unavailable")
	}
	ch := make(chan StreamEvent, len(f.events))
	go func() {
		defer close(ch)
		for _, e := range f.events {
			ch <- e
		}
	}()
	return ch, nil
}
