package worker

import (
	"context"
	"log"
	"sync"

	"github.com/WilliamLi0623/codex-issue-worker/internal/issue"
)

type Status string

const (
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Skipped   Status = "skipped"
)

type Result struct {
	Issue   int
	Status  Status
	Branch  string
	PRURL   string
	LogPath string
	Err     error
}
type Runner interface {
	Run(context.Context, issue.Issue) Result
}

type Pool struct {
	sem    chan struct{}
	runner Runner
	wg     sync.WaitGroup
	mu     sync.Mutex
	active map[int]struct{}
}

func NewPool(limit int, r Runner) *Pool {
	if limit < 1 {
		panic("worker pool limit must be positive")
	}
	if r == nil {
		panic("worker pool runner must not be nil")
	}
	return &Pool{sem: make(chan struct{}, limit), runner: r, active: map[int]struct{}{}}
}
func (p *Pool) Submit(ctx context.Context, i issue.Issue) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.active[i.Number]; exists {
		return false
	}
	select {
	case p.sem <- struct{}{}:
	default:
		return false
	}
	p.active[i.Number] = struct{}{}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer func() { <-p.sem; p.mu.Lock(); delete(p.active, i.Number); p.mu.Unlock() }()
		result := p.runner.Run(ctx, i)
		log.Printf("issue=%d status=%s branch=%s pr=%s log=%s err=%v", result.Issue, result.Status, result.Branch, result.PRURL, result.LogPath, result.Err)
	}()
	return true
}
func (p *Pool) Wait() { p.wg.Wait() }
