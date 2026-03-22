package mvp

import (
	"context"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

func TestOrchestratorStartPause(t *testing.T) {
	o := &Orchestrator{paused: true}

	if !o.IsPaused() {
		t.Error("new orchestrator should be paused")
	}

	o.Start()
	if o.IsPaused() {
		t.Error("orchestrator should not be paused after Start()")
	}

	o.Pause()
	if !o.IsPaused() {
		t.Error("orchestrator should be paused after Pause()")
	}
}

func TestOrchestratorIsProcessing(t *testing.T) {
	o := &Orchestrator{}

	if o.IsProcessing() {
		t.Error("new orchestrator should not be processing")
	}

	o.mu.Lock()
	o.processing = true
	o.mu.Unlock()

	if !o.IsProcessing() {
		t.Error("orchestrator should be processing")
	}
}

func TestOrchestratorCurrentTask(t *testing.T) {
	o := &Orchestrator{}

	if o.CurrentTask() != nil {
		t.Error("new orchestrator should have nil current task")
	}

	task := &Task{
		Issue:  github.Issue{Number: 42, Title: "Test"},
		Status: StatusCoding,
	}

	o.mu.Lock()
	o.currentTask = task
	o.mu.Unlock()

	got := o.CurrentTask()
	if got == nil {
		t.Fatal("expected non-nil current task")
	}
	if got.Issue.Number != 42 {
		t.Errorf("CurrentTask().Issue.Number = %d, want 42", got.Issue.Number)
	}
}

func TestOrchestratorStop(t *testing.T) {
	o := &Orchestrator{running: true}

	o.Stop()

	o.mu.Lock()
	running := o.running
	o.mu.Unlock()

	if running {
		t.Error("orchestrator should not be running after Stop()")
	}
}

func TestOrchestratorRunContextCancel(t *testing.T) {
	o := &Orchestrator{paused: true}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- o.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancel")
	}
}

func TestOrchestratorRunStop(t *testing.T) {
	o := &Orchestrator{paused: true}

	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		done <- o.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	o.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after Stop()")
	}
}

func TestHasLabel(t *testing.T) {
	issue := github.Issue{
		Labels: []struct {
			Name string `json:"name"`
		}{
			{Name: "in-progress"},
			{Name: "bug"},
		},
	}

	if !hasLabel(issue, "in-progress") {
		t.Error("expected hasLabel to find 'in-progress'")
	}
	if !hasLabel(issue, "bug") {
		t.Error("expected hasLabel to find 'bug'")
	}
	if hasLabel(issue, "feature") {
		t.Error("expected hasLabel to not find 'feature'")
	}
}

func TestHasLabelEmpty(t *testing.T) {
	issue := github.Issue{}
	if hasLabel(issue, "any") {
		t.Error("expected hasLabel to return false for empty labels")
	}
}
