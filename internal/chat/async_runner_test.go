package chat

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAsyncRunner_SupersededTaskSkipsStaleCommit(t *testing.T) {
	runner := NewAsyncRunner(AsyncRunnerConfig{
		Workers:   1,
		QueueSize: 2,
		Timeout:   time.Second,
	}, nil)

	startFirst := make(chan struct{})
	releaseFirst := make(chan struct{})

	var mu sync.Mutex
	committed := make([]string, 0, 2)

	recordCommit := func(label string) func(context.Context) error {
		return func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			committed = append(committed, label)
			return nil
		}
	}

	runner.Enqueue(AsyncTask{
		Key:     "user:1",
		Version: 1,
		Build: func(ctx context.Context) (func(context.Context) error, error) {
			close(startFirst)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return recordCommit("stale"), nil
		},
	})

	<-startFirst

	runner.Enqueue(AsyncTask{
		Key:     "user:1",
		Version: 2,
		Build: func(context.Context) (func(context.Context) error, error) {
			return recordCommit("fresh"), nil
		},
	})

	close(releaseFirst)
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(committed) != 1 || committed[0] != "fresh" {
		t.Fatalf("expected only fresh commit to survive, got %#v", committed)
	}
}

func TestAsyncRunner_QueueFullDropsTask(t *testing.T) {
	runner := NewAsyncRunner(AsyncRunnerConfig{
		Workers:   1,
		QueueSize: 1,
		Timeout:   time.Second,
	}, nil)

	block := make(chan struct{})
	dropped := make(chan struct{}, 1)

	runner.Enqueue(AsyncTask{
		Key:     "user:1",
		Version: 1,
		Build: func(ctx context.Context) (func(context.Context) error, error) {
			select {
			case <-block:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return nil, nil
		},
	})

	runner.Enqueue(AsyncTask{
		Key:     "user:2",
		Version: 1,
		Build: func(context.Context) (func(context.Context) error, error) {
			return nil, nil
		},
	})

	runner.Enqueue(AsyncTask{
		Key:     "user:3",
		Version: 1,
		Build: func(context.Context) (func(context.Context) error, error) {
			return nil, nil
		},
		OnDrop: func(reason string, _ int) {
			if reason == "queue_full" {
				dropped <- struct{}{}
			}
		},
	})

	select {
	case <-dropped:
	case <-time.After(time.Second):
		t.Fatal("expected queue full drop callback")
	}

	close(block)
}
