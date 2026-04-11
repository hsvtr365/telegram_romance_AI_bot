package chat

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type AsyncRunnerConfig struct {
	Workers   int
	QueueSize int
	Timeout   time.Duration
}

type AsyncTask struct {
	Key     string
	Version int64

	Build func(ctx context.Context) (func(context.Context) error, error)

	OnEnqueue    func(queueDepth int)
	OnDrop       func(reason string, queueDepth int)
	OnSuperseded func(stage string)
	OnComplete   func(duration time.Duration, err error, queueDepth int)
}

type AsyncRunner struct {
	cfg    AsyncRunnerConfig
	logger *slog.Logger

	jobs chan AsyncTask

	mu     sync.Mutex
	latest map[string]int64
}

func NewAsyncRunner(cfg AsyncRunnerConfig, logger *slog.Logger) *AsyncRunner {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 16
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	runner := &AsyncRunner{
		cfg:    cfg,
		logger: logger,
		jobs:   make(chan AsyncTask, cfg.QueueSize),
		latest: make(map[string]int64),
	}

	for i := 0; i < cfg.Workers; i++ {
		go runner.worker()
	}

	return runner
}

func (r *AsyncRunner) Enqueue(task AsyncTask) bool {
	if r == nil || task.Build == nil {
		return false
	}

	if task.Key != "" && task.Version > 0 {
		r.mu.Lock()
		if task.Version > r.latest[task.Key] {
			r.latest[task.Key] = task.Version
		}
		r.mu.Unlock()
	}

	select {
	case r.jobs <- task:
		if task.OnEnqueue != nil {
			task.OnEnqueue(len(r.jobs))
		}
		return true
	default:
		if task.OnDrop != nil {
			task.OnDrop("queue_full", len(r.jobs))
		}
		return false
	}
}

func (r *AsyncRunner) worker() {
	for task := range r.jobs {
		if r.isSuperseded(task.Key, task.Version) {
			if task.OnSuperseded != nil {
				task.OnSuperseded("before_build")
			}
			continue
		}

		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), r.cfg.Timeout)
		commit, err := task.Build(ctx)
		if err == nil && commit != nil && !r.isSuperseded(task.Key, task.Version) {
			err = commit(ctx)
		} else if err == nil && commit != nil && task.OnSuperseded != nil {
			task.OnSuperseded("after_build")
		}
		cancel()

		if task.OnComplete != nil {
			task.OnComplete(time.Since(start), err, len(r.jobs))
		}
	}
}

func (r *AsyncRunner) isSuperseded(key string, version int64) bool {
	if r == nil || key == "" || version == 0 {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	latest := r.latest[key]
	return latest != 0 && version < latest
}
