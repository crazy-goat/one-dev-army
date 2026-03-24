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

func TestSendDecision_WorkerProcessing(t *testing.T) {
	o := &Orchestrator{
		worker: &Worker{decisionCh: make(chan UserDecision, 1)},
	}
	o.currentTask = &Task{
		Issue: github.Issue{Number: 42},
	}

	err := o.SendDecision(42, UserDecision{Action: "approve"})
	if err != nil {
		t.Fatalf("SendDecision() error = %v", err)
	}

	select {
	case d := <-o.worker.decisionCh:
		if d.Action != "approve" {
			t.Errorf("decision.Action = %q, want %q", d.Action, "approve")
		}
	default:
		t.Fatal("expected decision on channel")
	}
}

func TestSendDecision_NoWorker(t *testing.T) {
	o := &Orchestrator{
		worker: &Worker{decisionCh: make(chan UserDecision, 1)},
	}

	err := o.SendDecision(42, UserDecision{Action: "approve"})
	if err == nil {
		t.Fatal("expected error when no worker processing")
	}
}

func TestSendDecision_WrongIssue(t *testing.T) {
	o := &Orchestrator{
		worker: &Worker{decisionCh: make(chan UserDecision, 1)},
	}
	o.currentTask = &Task{
		Issue: github.Issue{Number: 42},
	}

	err := o.SendDecision(99, UserDecision{Action: "approve"})
	if err == nil {
		t.Fatal("expected error for wrong issue number")
	}
}

func TestSendDecision_Decline(t *testing.T) {
	o := &Orchestrator{
		worker: &Worker{decisionCh: make(chan UserDecision, 1)},
	}
	o.currentTask = &Task{
		Issue: github.Issue{Number: 42},
	}

	err := o.SendDecision(42, UserDecision{Action: "decline", Reason: "needs more tests"})
	if err != nil {
		t.Fatalf("SendDecision() error = %v", err)
	}

	select {
	case d := <-o.worker.decisionCh:
		if d.Action != "decline" {
			t.Errorf("decision.Action = %q, want %q", d.Action, "decline")
		}
		if d.Reason != "needs more tests" {
			t.Errorf("decision.Reason = %q, want %q", d.Reason, "needs more tests")
		}
	default:
		t.Fatal("expected decision on channel")
	}
}

func TestGetStageLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   string
	}{
		{"no labels", nil, ""},
		{"no stage labels", []string{"bug", "priority:high"}, ""},
		{"stage:coding", []string{"stage:coding"}, "stage:coding"},
		{"mixed with stage", []string{"bug", "stage:merging"}, "stage:merging"},
		{"stage:awaiting-approval", []string{"stage:awaiting-approval"}, "stage:awaiting-approval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := github.Issue{}
			for _, l := range tt.labels {
				issue.Labels = append(issue.Labels, struct {
					Name string `json:"name"`
				}{Name: l})
			}
			got := getStageLabel(issue)
			if got != tt.want {
				t.Errorf("getStageLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsWorkerStage(t *testing.T) {
	workerStages := []string{
		"stage:analysis", "stage:coding", "stage:code-review",
		"stage:create-pr", "stage:awaiting-approval",
	}
	for _, s := range workerStages {
		if !isWorkerStage(s) {
			t.Errorf("isWorkerStage(%q) = false, want true", s)
		}
	}

	nonWorkerStages := []string{
		"stage:merging", "stage:done", "stage:failed",
		"stage:blocked", "stage:needs-user", "stage:backlog", "",
	}
	for _, s := range nonWorkerStages {
		if isWorkerStage(s) {
			t.Errorf("isWorkerStage(%q) = true, want false", s)
		}
	}
}

func TestDecideNextStage_MergeSuccess(t *testing.T) {
	o := &Orchestrator{}
	event := WorkerEvent{
		IssueNumber: 42,
		Stage:       "merge",
		Status:      EventSuccess,
	}

	stage, reason, ok := o.decideNextStage(event)
	if !ok {
		t.Fatal("expected transition for merge success")
	}
	if stage != github.StageDone {
		t.Errorf("stage = %q, want %q", stage, github.StageDone)
	}
	if reason != github.ReasonWorkerCompletedMerge {
		t.Errorf("reason = %q, want %q", reason, github.ReasonWorkerCompletedMerge)
	}
}

func TestDecideNextStage_AwaitingApprovalSuccess(t *testing.T) {
	o := &Orchestrator{}
	event := WorkerEvent{
		IssueNumber: 42,
		Stage:       "awaiting-approval",
		Status:      EventSuccess,
	}

	stage, reason, ok := o.decideNextStage(event)
	if !ok {
		t.Fatal("expected transition for awaiting-approval success")
	}
	if stage != github.StageMerge {
		t.Errorf("stage = %q, want %q", stage, github.StageMerge)
	}
	if reason != github.ReasonManualMerge {
		t.Errorf("reason = %q, want %q", reason, github.ReasonManualMerge)
	}
}
