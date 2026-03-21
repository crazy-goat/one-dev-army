package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/one-dev-army/oda/internal/worker"
)

type mockQueue struct {
	mu      sync.Mutex
	tasks   []*worker.Task
	done    []int
	blocked map[int]string
	idx     int
}

func newMockQueue(tasks []*worker.Task) *mockQueue {
	return &mockQueue{
		tasks:   tasks,
		blocked: make(map[int]string),
	}
}

func (q *mockQueue) Next() (*worker.Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.idx >= len(q.tasks) {
		return nil, nil
	}
	t := q.tasks[q.idx]
	q.idx++
	return t, nil
}

func (q *mockQueue) MarkDone(taskID int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.done = append(q.done, taskID)
	return nil
}

func (q *mockQueue) MarkBlocked(taskID int, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.blocked[taskID] = reason
	return nil
}

type mockProcessor struct {
	mu        sync.Mutex
	processed []int
}

func (p *mockProcessor) Process(_ context.Context, w *worker.Worker, task *worker.Task) error {
	w.SetStage("processing")
	time.Sleep(5 * time.Millisecond)
	p.mu.Lock()
	p.processed = append(p.processed, task.ID)
	p.mu.Unlock()
	return nil
}

func TestPoolProcessesTasks(t *testing.T) {
	tasks := []*worker.Task{
		{ID: 1, Title: "task-1"},
		{ID: 2, Title: "task-2"},
		{ID: 3, Title: "task-3"},
	}
	queue := newMockQueue(tasks)
	proc := &mockProcessor{}

	pool := worker.NewPool(2, queue, proc)
	pool.Start(context.Background())
	pool.Wait()

	proc.mu.Lock()
	defer proc.mu.Unlock()
	if len(proc.processed) != 3 {
		t.Fatalf("processed %d tasks, want 3", len(proc.processed))
	}

	seen := make(map[int]bool)
	for _, id := range proc.processed {
		seen[id] = true
	}
	for _, task := range tasks {
		if !seen[task.ID] {
			t.Errorf("task %d was not processed", task.ID)
		}
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.done) != 3 {
		t.Fatalf("marked done %d tasks, want 3", len(queue.done))
	}
}

func TestPoolWorkerInfo(t *testing.T) {
	queue := newMockQueue(nil)
	proc := &mockProcessor{}

	pool := worker.NewPool(3, queue, proc)
	infos := pool.Workers()

	if len(infos) != 3 {
		t.Fatalf("got %d workers, want 3", len(infos))
	}

	for i, info := range infos {
		if info.Status != worker.StatusIdle {
			t.Errorf("worker %d status = %q, want %q", i, info.Status, worker.StatusIdle)
		}
		if info.TaskID != 0 {
			t.Errorf("worker %d taskID = %d, want 0", i, info.TaskID)
		}
	}
}

func TestPoolContextCancel(t *testing.T) {
	slowProc := &slowProcessor{delay: 500 * time.Millisecond}
	tasks := make([]*worker.Task, 100)
	for i := range tasks {
		tasks[i] = &worker.Task{ID: i + 1, Title: "task"}
	}
	queue := newMockQueue(tasks)

	ctx, cancel := context.WithCancel(context.Background())
	pool := worker.NewPool(2, queue, slowProc)
	pool.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		pool.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pool did not stop within timeout after context cancel")
	}

	queue.mu.Lock()
	processed := len(queue.done)
	queue.mu.Unlock()
	if processed >= len(tasks) {
		t.Errorf("expected fewer than %d tasks processed after cancel, got %d", len(tasks), processed)
	}
}

func TestPoolEmptyQueue(t *testing.T) {
	queue := newMockQueue(nil)
	proc := &mockProcessor{}

	pool := worker.NewPool(2, queue, proc)
	pool.Start(context.Background())

	done := make(chan struct{})
	go func() {
		pool.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pool did not finish with empty queue within timeout")
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()
	if len(proc.processed) != 0 {
		t.Errorf("processed %d tasks, want 0", len(proc.processed))
	}
}

type slowProcessor struct {
	delay time.Duration
}

func (p *slowProcessor) Process(ctx context.Context, _ *worker.Worker, _ *worker.Task) error {
	select {
	case <-time.After(p.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
