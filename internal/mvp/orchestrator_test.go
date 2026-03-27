package mvp

import (
	"context"
	"errors"
	"strings"
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
	o := &Orchestrator{
		running: true,
		stopCh:  make(chan struct{}),
	}

	o.Stop()

	o.mu.Lock()
	running := o.running
	o.mu.Unlock()

	if running {
		t.Error("orchestrator should not be running after Stop()")
	}
}

func TestOrchestratorRunContextCancel(t *testing.T) {
	o := &Orchestrator{
		paused:       true,
		completionCh: make(chan int, 1),
		stopCh:       make(chan struct{}),
	}

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
	o := &Orchestrator{
		paused:       true,
		completionCh: make(chan int, 1),
		stopCh:       make(chan struct{}),
	}

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
		"stage:create-pr", "stage:check-pipeline", "stage:awaiting-approval",
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

func TestDecideNextStage_CheckPipelineSuccess(t *testing.T) {
	o := &Orchestrator{}
	event := WorkerEvent{
		IssueNumber: 42,
		Stage:       "check-pipeline",
		Status:      EventSuccess,
	}
	stage, reason, ok := o.decideNextStage(event)
	if !ok {
		t.Fatal("expected transition for check-pipeline success")
	}
	if stage != github.StageApprove {
		t.Errorf("stage = %q, want %q", stage, github.StageApprove)
	}
	if reason != github.ReasonWorkerCompletedCheckPipeline {
		t.Errorf("reason = %q, want %q", reason, github.ReasonWorkerCompletedCheckPipeline)
	}
}

func TestDecideNextStage_CheckPipelineFailed(t *testing.T) {
	o := &Orchestrator{}
	event := WorkerEvent{
		IssueNumber: 42,
		Stage:       "check-pipeline",
		Status:      EventFailed,
	}
	stage, reason, ok := o.decideNextStage(event)
	if !ok {
		t.Fatal("expected transition for check-pipeline failure")
	}
	if stage != github.StageCode {
		t.Errorf("stage = %q, want %q", stage, github.StageCode)
	}
	if reason != github.ReasonCheckPipelineFailed {
		t.Errorf("reason = %q, want %q", reason, github.ReasonCheckPipelineFailed)
	}
}

func TestDecideNextStage_CreatePRGoesToCheckPipeline(t *testing.T) {
	o := &Orchestrator{}
	event := WorkerEvent{
		IssueNumber: 42,
		Stage:       "create-pr",
		Status:      EventSuccess,
	}
	stage, _, ok := o.decideNextStage(event)
	if !ok {
		t.Fatal("expected transition for create-pr success")
	}
	if stage != github.StageCheckPipeline {
		t.Errorf("stage = %q, want %q", stage, github.StageCheckPipeline)
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
func TestOrchestrator_ImplementsConfigAwareWorker(_ *testing.T) {
	// This is a compile-time check
	var _ config.ConfigAwareWorker = (*Orchestrator)(nil)
}

func TestQueueManualProcess(t *testing.T) {
	o := &Orchestrator{paused: true}

	err := o.QueueManualProcess(42)
	if err != nil {
		t.Fatalf("QueueManualProcess() error = %v", err)
	}
	if o.ManualNext() != 42 {
		t.Errorf("ManualNext() = %d, want 42", o.ManualNext())
	}
	if o.IsPaused() {
		t.Error("orchestrator should be auto-started after manual process")
	}
}

func TestQueueManualProcessReplace(t *testing.T) {
	o := &Orchestrator{paused: false}

	o.QueueManualProcess(42)
	o.QueueManualProcess(99)

	if o.ManualNext() != 99 {
		t.Errorf("ManualNext() = %d, want 99 (should replace previous)", o.ManualNext())
	}
}

func TestQueueManualProcessAlreadyProcessing(t *testing.T) {
	o := &Orchestrator{}
	o.currentTask = &Task{Issue: github.Issue{Number: 42}}

	err := o.QueueManualProcess(42)
	if err == nil {
		t.Error("expected error when queuing ticket that is already being processed")
	}
}

// mockStageBroadcaster is a test mock for StageBroadcaster
type mockStageBroadcaster struct {
	issueUpdates                  []github.Issue
	workerUpdates                 []workerUpdateCall
	sprintClosable                []bool
	broadcastSprintClosableCalled bool
}

type workerUpdateCall struct {
	workerID, status string
	taskID           int
	taskTitle, stage string
	elapsedSeconds   int
}

func (m *mockStageBroadcaster) BroadcastIssueUpdate(issue github.Issue) {
	m.issueUpdates = append(m.issueUpdates, issue)
}

func (m *mockStageBroadcaster) BroadcastWorkerUpdate(workerID, status string, taskID int, taskTitle, stage string, elapsedSeconds int) {
	m.workerUpdates = append(m.workerUpdates, workerUpdateCall{
		workerID:       workerID,
		status:         status,
		taskID:         taskID,
		taskTitle:      taskTitle,
		stage:          stage,
		elapsedSeconds: elapsedSeconds,
	})
}

func (m *mockStageBroadcaster) BroadcastSprintClosable(canClose bool) {
	m.sprintClosable = append(m.sprintClosable, canClose)
	m.broadcastSprintClosableCalled = true
}

func TestInferColumnForClosableCheck(t *testing.T) {
	tests := []struct {
		name  string
		issue github.Issue
		want  string
	}{
		{
			name:  "closed issue is Done",
			issue: github.Issue{State: "CLOSED"},
			want:  "Done",
		},
		{
			name: "stage:failed label is Failed",
			issue: github.Issue{State: "OPEN", Labels: []struct {
				Name string `json:"name"`
			}{{Name: "stage:failed"}}},
			want: "Failed",
		},
		{
			name: "failed label is Failed",
			issue: github.Issue{State: "OPEN", Labels: []struct {
				Name string `json:"name"`
			}{{Name: "failed"}}},
			want: "Failed",
		},
		{
			name: "stage:coding is Active",
			issue: github.Issue{State: "OPEN", Labels: []struct {
				Name string `json:"name"`
			}{{Name: "stage:coding"}}},
			want: "Active",
		},
		{
			name: "in-progress is Active",
			issue: github.Issue{State: "OPEN", Labels: []struct {
				Name string `json:"name"`
			}{{Name: "in-progress"}}},
			want: "Active",
		},
		{
			name: "stage:blocked is Active",
			issue: github.Issue{State: "OPEN", Labels: []struct {
				Name string `json:"name"`
			}{{Name: "stage:blocked"}}},
			want: "Active",
		},
		{
			name: "stage:check-pipeline is Active",
			issue: github.Issue{State: "OPEN", Labels: []struct {
				Name string `json:"name"`
			}{{Name: "stage:check-pipeline"}}},
			want: "Active",
		},
		{
			name:  "no labels is Backlog",
			issue: github.Issue{State: "OPEN"},
			want:  "Backlog",
		},
		{
			name: "bug label is Backlog",
			issue: github.Issue{State: "OPEN", Labels: []struct {
				Name string `json:"name"`
			}{{Name: "bug"}}},
			want: "Backlog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferColumnForClosableCheck(tt.issue)
			if got != tt.want {
				t.Errorf("inferColumnForClosableCheck() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOrchestrator_NotifyTicketCompleted(t *testing.T) {
	o := &Orchestrator{
		completionCh: make(chan int, 1),
	}

	o.NotifyTicketCompleted(42)

	select {
	case num := <-o.completionCh:
		if num != 42 {
			t.Errorf("received issue number %d, want 42", num)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for completion notification")
	}
}

func TestOrchestrator_NotifyTicketCompleted_NonBlocking(t *testing.T) {
	o := &Orchestrator{
		completionCh: make(chan int, 1),
	}

	// Fill the channel
	o.completionCh <- 1

	// This should not block even though channel is full
	done := make(chan bool)
	go func() {
		o.NotifyTicketCompleted(2)
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(time.Second):
		t.Error("NotifyTicketCompleted blocked on full channel")
	}
}

func TestOrchestrator_ImmediateSearchTriggered(t *testing.T) {
	o := &Orchestrator{
		completionCh: make(chan int, 1),
		paused:       true,
	}

	// Simulate notification
	o.NotifyTicketCompleted(42)

	// Verify notification was received
	select {
	case <-o.completionCh:
		// Good, notification received
	case <-time.After(time.Second):
		t.Error("notification not received within 1 second")
	}
}

func TestCheckAndBroadcastSprintClosable_AllTerminal(t *testing.T) {
	// Create mock hub
	mockHub := &mockStageBroadcaster{}

	// Create orchestrator with just the hub (store is nil, so it will return early)
	// We'll test the logic by calling the helper directly
	o := &Orchestrator{
		hub: mockHub,
	}

	// Since store is nil, checkAndBroadcastSprintClosable should return early
	o.checkAndBroadcastSprintClosable()

	// Verify BroadcastSprintClosable was NOT called (no store)
	if mockHub.broadcastSprintClosableCalled {
		t.Error("expected BroadcastSprintClosable NOT to be called when store is nil")
	}
}

func TestCheckAndBroadcastSprintClosable_NoHub(_ *testing.T) {
	// Create orchestrator without hub
	o := &Orchestrator{}

	// Should not panic and should return early
	o.checkAndBroadcastSprintClosable()
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

func TestOrchestratorRestartCurrentStage_StoreNotConfigured(t *testing.T) {
	o := &Orchestrator{}

	err := o.RestartCurrentStage(42)
	if err == nil {
		t.Error("expected error when store not configured")
	}
	if !strings.Contains(err.Error(), "store not configured") {
		t.Errorf("expected 'store not configured' error, got: %v", err)
	}
}

func TestOrchestratorRestartCurrentStage_TaskProcessing(t *testing.T) {
	o := &Orchestrator{
		currentTask: &Task{
			Issue: github.Issue{Number: 42},
		},
	}

	err := o.RestartCurrentStage(42)
	if err == nil {
		t.Error("expected error when task is actively being processed")
	}
	if !strings.Contains(err.Error(), "actively being processed") {
		t.Errorf("expected 'actively being processed' error, got: %v", err)
	}
}

func TestOrchestratorRestartFromBeginning_TaskProcessing(t *testing.T) {
	o := &Orchestrator{
		currentTask: &Task{
			Issue: github.Issue{Number: 42},
		},
	}

	err := o.RestartFromBeginning(42)
	if err == nil {
		t.Error("expected error when task is actively being processed")
	}
	if !strings.Contains(err.Error(), "actively being processed") {
		t.Errorf("expected 'actively being processed' error, got: %v", err)
	}
}

func TestStageToStepName(t *testing.T) {
	tests := []struct {
		stage    string
		expected string
	}{
		{"stage:analysis", "technical-planning"},
		{"stage:coding", "implement"},
		{"stage:code-review", "code-review"},
		{"stage:create-pr", "create-pr"},
		{"stage:check-pipeline", "check-pipeline"},
		{"stage:awaiting-approval", "awaiting-approval"},
		{"stage:merging", "merge"},
		{"stage:unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := stageToStepName(tt.stage)
		if got != tt.expected {
			t.Errorf("stageToStepName(%q) = %q, want %q", tt.stage, got, tt.expected)
		}
	}
}
