package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WilliamLi0623/codex-issue-worker/internal/issue"
)

type blockingRunner struct {
	active  int32
	peak    int32
	release <-chan struct{}
	wg      *sync.WaitGroup
}

func (r *blockingRunner) Run(ctx context.Context, i issue.Issue) Result {
	if r.wg != nil {
		defer r.wg.Done()
	}
	n := atomic.AddInt32(&r.active, 1)
	for {
		p := atomic.LoadInt32(&r.peak)
		if n <= p || atomic.CompareAndSwapInt32(&r.peak, p, n) {
			break
		}
	}
	defer atomic.AddInt32(&r.active, -1)
	select {
	case <-r.release:
		return Result{Issue: i.Number, Status: Succeeded}
	case <-ctx.Done():
		return Result{Issue: i.Number, Status: Failed, Err: ctx.Err()}
	}
}

func TestPoolBoundsConcurrency(t *testing.T) {
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	r := &blockingRunner{release: release, wg: &wg}
	p := NewPool(2, r)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for n := 1; n <= 2; n++ {
		if !p.Submit(ctx, issue.Issue{Number: n}) {
			t.Fatalf("issue %d rejected", n)
		}
	}
	if p.Submit(ctx, issue.Issue{Number: 3}) {
		t.Fatal("accepted work beyond the concurrency limit")
	}
	if p.Submit(ctx, issue.Issue{Number: 1}) {
		t.Fatal("accepted duplicate issue")
	}
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&r.peak); got != 2 {
		t.Fatalf("peak=%d want 2", got)
	}
	close(release)
	wg.Wait()
	p.Wait()
}

func TestPoolCancellationReachesTasks(t *testing.T) {
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	r := &blockingRunner{release: release, wg: &wg}
	p := NewPool(1, r)
	ctx, cancel := context.WithCancel(context.Background())
	p.Submit(ctx, issue.Issue{Number: 1})
	cancel()
	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pool did not stop after cancellation")
	}
}

func TestPoolRejectsInvalidLimit(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid limit to panic")
		}
	}()
	NewPool(0, &blockingRunner{})
}
