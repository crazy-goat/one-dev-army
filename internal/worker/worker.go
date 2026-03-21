package worker

import (
	"sync"
	"time"
)

type Status string

const (
	StatusIdle    Status = "idle"
	StatusWorking Status = "working"
)

type Task struct {
	ID           int
	IssueNumber  int
	Title        string
	Body         string
	Stage        string
	Dependencies []int
}

type WorkerInfo struct {
	ID        string
	Status    Status
	TaskID    int
	TaskTitle string
	Stage     string
	Elapsed   time.Duration
}

type Worker struct {
	mu        sync.RWMutex
	id        string
	status    Status
	taskID    int
	taskTitle string
	stage     string
	startTime time.Time
}

func NewWorker(id string) *Worker {
	return &Worker{
		id:     id,
		status: StatusIdle,
	}
}

func (w *Worker) SetTask(task *Task) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status = StatusWorking
	w.taskID = task.ID
	w.taskTitle = task.Title
	w.stage = task.Stage
	w.startTime = time.Now()
}

func (w *Worker) SetStage(stage string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stage = stage
}

func (w *Worker) SetIdle() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status = StatusIdle
	w.taskID = 0
	w.taskTitle = ""
	w.stage = ""
	w.startTime = time.Time{}
}

func (w *Worker) GetInfo() WorkerInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var elapsed time.Duration
	if !w.startTime.IsZero() {
		elapsed = time.Since(w.startTime)
	}
	return WorkerInfo{
		ID:        w.id,
		Status:    w.status,
		TaskID:    w.taskID,
		TaskTitle: w.taskTitle,
		Stage:     w.stage,
		Elapsed:   elapsed,
	}
}
