package worker

import (
	"context"
	"fmt"
	"sync"
)

type TaskQueue interface {
	Next() (*Task, error)
	MarkDone(taskID int) error
	MarkBlocked(taskID int, reason string) error
}

type EmptyQueue struct{}

func (*EmptyQueue) Next() (*Task, error)              { return nil, nil }
func (*EmptyQueue) MarkDone(_ int) error              { return nil }
func (*EmptyQueue) MarkBlocked(_ int, _ string) error { return nil }

type TaskProcessor interface {
	Process(ctx context.Context, worker *Worker, task *Task) error
}

type Pool struct {
	workers   []*Worker
	queue     TaskQueue
	processor TaskProcessor
	wg        sync.WaitGroup
}

func NewPool(count int, queue TaskQueue, processor TaskProcessor) *Pool {
	workers := make([]*Worker, count)
	for i := range count {
		workers[i] = NewWorker(fmt.Sprintf("worker-%d", i+1))
	}
	return &Pool{
		workers:   workers,
		queue:     queue,
		processor: processor,
	}
}

func (p *Pool) Start(ctx context.Context) {
	for _, w := range p.workers {
		p.wg.Add(1)
		go p.run(ctx, w)
	}
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

func (p *Pool) Workers() []WorkerInfo {
	infos := make([]WorkerInfo, len(p.workers))
	for i, w := range p.workers {
		infos[i] = w.GetInfo()
	}
	return infos
}

func (p *Pool) run(ctx context.Context, w *Worker) {
	defer p.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}

		task, err := p.queue.Next()
		if err != nil {
			return
		}
		if task == nil {
			return
		}

		w.SetTask(task)
		err = p.processor.Process(ctx, w, task)
		if err != nil {
			_ = p.queue.MarkBlocked(task.ID, err.Error())
		} else {
			_ = p.queue.MarkDone(task.ID)
		}
		w.SetIdle()
	}
}
