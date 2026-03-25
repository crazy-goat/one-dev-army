package mvp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
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
		if !errors.Is(err, context.Canceled) {
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

// TestResumeIssueClassification verifies that issues with worker stage labels
// are classified as resume issues (not backlog candidates), which is critical
// to prevent the orchestrator from resetting their stage to analysis.
func TestResumeIssueClassification(t *testing.T) {
	// Simulate the classification logic from Run()
	makeIssue := func(number int, labels ...string) github.Issue {
		issue := github.Issue{Number: number, State: "OPEN"}
		for _, l := range labels {
			issue.Labels = append(issue.Labels, struct {
				Name string `json:"name"`
			}{Name: l})
		}
		return issue
	}

	issues := []github.Issue{
		makeIssue(1),                            // no stage = backlog candidate
		makeIssue(2, "stage:coding"),            // worker stage = resume
		makeIssue(3, "stage:failed"),            // non-worker stage = ignored
		makeIssue(4, "stage:awaiting-approval"), // worker stage = resume (but #2 takes priority)
		makeIssue(5, "bug"),                     // no stage label = backlog candidate
		makeIssue(6, "stage:backlog"),           // stage:backlog = backlog candidate
	}

	var candidates []github.Issue
	var resumeIssue *github.Issue
	for i := range issues {
		stage := getStageLabel(issues[i])
		if stage == "" || stage == string(github.StageBacklog) {
			candidates = append(candidates, issues[i])
		} else if isWorkerStage(stage) && resumeIssue == nil {
			resumeIssue = &issues[i]
		}
	}

	// Should have 3 backlog candidates (#1, #5, and #6)
	if len(candidates) != 3 {
		t.Errorf("candidates count = %d, want 3", len(candidates))
	}
	if candidates[0].Number != 1 || candidates[1].Number != 5 || candidates[2].Number != 6 {
		t.Errorf("candidates = [#%d, #%d, #%d], want [#1, #5, #6]", candidates[0].Number, candidates[1].Number, candidates[2].Number)
	}

	// Resume issue should be #2 (first worker stage found)
	if resumeIssue == nil {
		t.Fatal("expected a resume issue, got nil")
	}
	if resumeIssue.Number != 2 {
		t.Errorf("resumeIssue.Number = %d, want 2", resumeIssue.Number)
	}

	// Key assertion: resume issue should NOT be treated as a new candidate.
	// The orchestrator must preserve its existing stage label, not reset to analysis.
	isResume := resumeIssue != nil
	if !isResume {
		t.Error("expected isResume=true for issue with stage:coding label")
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

func TestDecideNextStage_CodingInProgress(t *testing.T) {
	o := &Orchestrator{}
	event := WorkerEvent{
		IssueNumber: 42,
		Stage:       "coding",
		Status:      EventInProgress,
	}

	stage, reason, ok := o.decideNextStage(event)
	if !ok {
		t.Fatal("expected transition for coding in-progress")
	}
	if stage != github.StageCode {
		t.Errorf("stage = %q, want %q", stage, github.StageCode)
	}
	if reason != github.ReasonWorkerFixingFromReview {
		t.Errorf("reason = %q, want %q", reason, github.ReasonWorkerFixingFromReview)
	}
}

// TestStageBacklogAsCandidate verifies that issues explicitly labeled
// stage:backlog are treated as backlog candidates, not ignored.
func TestStageBacklogAsCandidate(t *testing.T) {
	makeIssue := func(number int, labels ...string) github.Issue {
		issue := github.Issue{Number: number, State: "OPEN"}
		for _, l := range labels {
			issue.Labels = append(issue.Labels, struct {
				Name string `json:"name"`
			}{Name: l})
		}
		return issue
	}

	// Single issue with stage:backlog label
	issues := []github.Issue{
		makeIssue(1, "stage:backlog"),
	}

	var candidates []github.Issue
	var resumeIssue *github.Issue
	for i := range issues {
		stage := getStageLabel(issues[i])
		if stage == "" || stage == string(github.StageBacklog) {
			candidates = append(candidates, issues[i])
		} else if isWorkerStage(stage) && resumeIssue == nil {
			resumeIssue = &issues[i]
		}
	}

	// Should be classified as a candidate
	if len(candidates) != 1 {
		t.Errorf("candidates count = %d, want 1", len(candidates))
	}
	if candidates[0].Number != 1 {
		t.Errorf("candidate issue number = %d, want 1", candidates[0].Number)
	}

	// Should NOT be a resume issue
	if resumeIssue != nil {
		t.Errorf("resumeIssue should be nil, got #%d", resumeIssue.Number)
	}
}

// TestOrchestrator_UpdateConfig verifies that UpdateConfig updates the orchestrator's config atomically.
func TestOrchestrator_UpdateConfig(t *testing.T) {
	initialCfg := &config.Config{
		YoloMode: false,
		LLM:      config.DefaultLLMConfig(),
	}

	// Create orchestrator manually to avoid nil client issues
	o := &Orchestrator{paused: true}
	o.cfg.Store(initialCfg)

	// Verify initial config
	if o.cfg.Load().YoloMode != false {
		t.Error("initial YoloMode should be false")
	}

	// Update config
	newCfg := &config.Config{
		YoloMode: true,
		LLM:      config.DefaultLLMConfig(),
	}
	newCfg.LLM.Code.Model = "new-code-model"

	o.UpdateConfig(newCfg)

	// Verify updated config
	if o.cfg.Load().YoloMode != true {
		t.Error("YoloMode should be true after UpdateConfig")
	}
	if o.cfg.Load().LLM.Code.Model != "new-code-model" {
		t.Errorf("Code.Model = %q, want %q", o.cfg.Load().LLM.Code.Model, "new-code-model")
	}
}

// TestOrchestrator_ImplementsConfigAwareWorker verifies that Orchestrator implements ConfigAwareWorker interface.
func TestOrchestrator_ImplementsConfigAwareWorker(t *testing.T) {
	// This is a compile-time check
	var _ config.ConfigAwareWorker = (*Orchestrator)(nil)
}

// TestOrchestrator_UpdateConfig_PropagatesToWorker verifies that UpdateConfig propagates to the worker.
func TestOrchestrator_UpdateConfig_PropagatesToWorker(t *testing.T) {
	initialCfg := &config.Config{
		YoloMode: false,
		LLM:      config.DefaultLLMConfig(),
	}

	// Create a worker manually
	worker := &Worker{id: 1}
	worker.cfg.Store(initialCfg)

	// Create orchestrator with the worker
	o := &Orchestrator{paused: true}
	o.cfg.Store(initialCfg)
	o.worker = worker

	// Verify worker has initial config
	if o.worker.cfg.Load().YoloMode != false {
		t.Error("worker initial YoloMode should be false")
	}

	// Update config via orchestrator
	newCfg := &config.Config{
		YoloMode: true,
		LLM:      config.DefaultLLMConfig(),
	}

	o.UpdateConfig(newCfg)

	// Verify worker received the update
	if o.worker.cfg.Load().YoloMode != true {
		t.Error("worker YoloMode should be true after orchestrator UpdateConfig")
	}
}
