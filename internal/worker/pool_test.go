package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/WilliamLi0623/codex-issue-worker/internal/issue"
)

type gatedRunner struct {
	active   int32
	peak     int32
	started  chan int
	release  chan struct{}
	canceled chan<- struct{}
}

func (r *gatedRunner) Run(ctx context.Context, i issue.Issue) Result {
	n := atomic.AddInt32(&r.active, 1)
	for {
		p := atomic.LoadInt32(&r.peak)
		if n <= p || atomic.CompareAndSwapInt32(&r.peak, p, n) {
			break
		}
	}
	defer atomic.AddInt32(&r.active, -1)
	r.started <- i.Number
	select {
	case <-r.release:
		return Result{Issue: i.Number, Status: Succeeded}
	case <-ctx.Done():
		if r.canceled != nil {
			r.canceled <- struct{}{}
		}
		return Result{Issue: i.Number, Status: Failed, Err: ctx.Err()}
	}
}

func TestPoolAcceptsConcurrentSubmissionsUpToLimit(t *testing.T) {
	const limit = 4
	r := &gatedRunner{started: make(chan int, limit), release: make(chan struct{})}
	p := NewPool(limit, r)

	accepted := make(chan bool, 100)
	var submissions sync.WaitGroup
	submissions.Add(cap(accepted))
	for n := 1; n <= cap(accepted); n++ {
		go func(number int) {
			defer submissions.Done()
			accepted <- p.Submit(context.Background(), issue.Issue{Number: number})
		}(n)
	}
	submissions.Wait()

	count := 0
	for range cap(accepted) {
		if <-accepted {
			count++
		}
	}
	if count != limit {
		t.Fatalf("accepted=%d want %d", count, limit)
	}
	for range limit {
		<-r.started
	}
	if got := atomic.LoadInt32(&r.peak); got != limit {
		t.Fatalf("peak=%d want %d", got, limit)
	}
	close(r.release)
	p.Wait()
}

func TestPoolSuppressesConcurrentDuplicateSubmissions(t *testing.T) {
	const attempts = 100
	r := &gatedRunner{started: make(chan int, 1), release: make(chan struct{})}
	p := NewPool(1, r)
	accepted := make(chan bool, attempts)
	var submissions sync.WaitGroup
	submissions.Add(attempts)
	for range attempts {
		go func() {
			defer submissions.Done()
			accepted <- p.Submit(context.Background(), issue.Issue{Number: 1})
		}()
	}
	submissions.Wait()

	count := 0
	for range attempts {
		if <-accepted {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("accepted duplicate submissions=%d want 1", count)
	}
	<-r.started
	close(r.release)
	p.Wait()
}

func TestPoolCancellationReachesTasks(t *testing.T) {
	canceled := make(chan struct{}, 1)
	r := &gatedRunner{started: make(chan int, 1), release: make(chan struct{}), canceled: canceled}
	p := NewPool(1, r)
	ctx, cancel := context.WithCancel(context.Background())
	if !p.Submit(ctx, issue.Issue{Number: 1}) {
		t.Fatal("submission was rejected")
	}
	<-r.started
	cancel()
	p.Wait()
	select {
	case <-canceled:
	default:
		t.Fatal("task did not observe cancellation")
	}
}

func TestPoolReleasesSlotAfterFailedTask(t *testing.T) {
	firstStarted := make(chan struct{})
	secondStarted := make(chan int, 1)
	var calls atomic.Int32
	r := runnerFunc(func(_ context.Context, i issue.Issue) Result {
		if calls.Add(1) == 1 {
			close(firstStarted)
			return Result{Issue: i.Number, Status: Failed}
		}
		secondStarted <- i.Number
		return Result{Issue: i.Number, Status: Succeeded}
	})
	p := NewPool(1, r)
	if !p.Submit(context.Background(), issue.Issue{Number: 1}) {
		t.Fatal("first submission was rejected")
	}
	<-firstStarted
	p.Wait()
	if !p.Submit(context.Background(), issue.Issue{Number: 2}) {
		t.Fatal("slot was not released after failed task")
	}
	if got := <-secondStarted; got != 2 {
		t.Fatalf("second task issue=%d want 2", got)
	}
	p.Wait()
}

func TestPoolBoundsHighSubmissionPressure(t *testing.T) {
	const (
		limit   = 8
		submits = 1000
	)
	r := &gatedRunner{started: make(chan int, limit), release: make(chan struct{})}
	p := NewPool(limit, r)
	accepted := make(chan bool, submits)
	var submissions sync.WaitGroup
	submissions.Add(submits)
	for n := 1; n <= submits; n++ {
		go func(number int) {
			defer submissions.Done()
			accepted <- p.Submit(context.Background(), issue.Issue{Number: number})
		}(n)
	}
	submissions.Wait()

	count := 0
	for range submits {
		if <-accepted {
			count++
		}
	}
	if count != limit {
		t.Fatalf("accepted=%d under pressure, want %d", count, limit)
	}
	for range limit {
		<-r.started
	}
	if got := atomic.LoadInt32(&r.peak); got > limit {
		t.Fatalf("peak=%d exceeds limit %d", got, limit)
	}
	close(r.release)
	p.Wait()
}

func TestPoolRejectsInvalidLimit(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid limit to panic")
		}
	}()
	NewPool(0, runnerFunc(func(context.Context, issue.Issue) Result { return Result{} }))
}

type runnerFunc func(context.Context, issue.Issue) Result

func (f runnerFunc) Run(ctx context.Context, i issue.Issue) Result { return f(ctx, i) }
