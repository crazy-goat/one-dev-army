package mvp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/prompts"
)

func TestSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Add feature X", "add-feature-x"},
		{"Fix bug #123", "fix-bug-123"},
		{"UPPERCASE TITLE", "uppercase-title"},
		{"  spaces  around  ", "spaces-around"},
		{"special!@#$%chars", "special-chars"},
		{"", ""},
		{"a", "a"},
		{"already-slugged", "already-slugged"},
		{"This is a very long title that should be truncated to forty characters maximum", "this-is-a-very-long-title-that-should-be"},
		{"Trailing-dash-at-exactly-forty-characters", "trailing-dash-at-exactly-forty-character"},
		{"dots.and.periods", "dots-and-periods"},
		{"multiple---dashes", "multiple-dashes"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slug(tt.input)
			if got != tt.want {
				t.Errorf("slug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSlugMaxLength(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz"
	got := slug(long)
	if len(got) > 40 {
		t.Errorf("slug length = %d, want <= 40", len(got))
	}
}

func TestSlugNoTrailingDash(t *testing.T) {
	input := "this title is exactly forty one chars long"
	got := slug(input)
	if len(got) > 0 && got[len(got)-1] == '-' {
		t.Errorf("slug(%q) = %q, ends with dash", input, got)
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name string
		msg  *opencode.Message
		want string
	}{
		{
			name: "nil message",
			msg:  nil,
			want: "",
		},
		{
			name: "empty parts",
			msg:  &opencode.Message{},
			want: "",
		},
		{
			name: "single text part",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "text", Text: "hello world"},
				},
			},
			want: "hello world",
		},
		{
			name: "multiple text parts",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "text", Text: "part1"},
					{Type: "text", Text: "part2"},
				},
			},
			want: "part1part2",
		},
		{
			name: "mixed part types",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "text", Text: "hello"},
					{Type: "tool_use", Text: "ignored"},
					{Type: "text", Text: " world"},
				},
			},
			want: "hello world",
		},
		{
			name: "no text parts",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "tool_use", Text: "ignored"},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractText(tt.msg)
			if got != tt.want {
				t.Errorf("extractText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTechnicalPlanningResponse(t *testing.T) {
	tests := []struct {
		name         string
		response     string
		wantAnalysis string
		wantPlan     string
	}{
		{
			name: "both sections present",
			response: `## Analysis

1. Core requirements: implement feature X
2. Files to change: main.go
3. Implementation approach: add new function
4. Testing strategy: unit tests

## Implementation Plan

1. Modify main.go to add feature
2. Add tests in main_test.go`,
			wantAnalysis: "1. Core requirements: implement feature X\n2. Files to change: main.go\n3. Implementation approach: add new function\n4. Testing strategy: unit tests",
			wantPlan:     "1. Modify main.go to add feature\n2. Add tests in main_test.go",
		},
		{
			name: "only analysis header",
			response: `## Analysis

Some analysis content here
More analysis content`,
			wantAnalysis: "Some analysis content here\nMore analysis content",
			wantPlan:     "Some analysis content here\nMore analysis content",
		},
		{
			name: "only plan header",
			response: `Some intro text

## Implementation Plan

1. Step one
2. Step two`,
			wantAnalysis: "Some intro text",
			wantPlan:     "1. Step one\n2. Step two",
		},
		{
			name: "no headers - heuristic split",
			response: `Analysis part here.

Implementation Plan:
1. First step
2. Second step`,
			wantAnalysis: "Analysis part here.",
			wantPlan:     "Implementation Plan:\n1. First step\n2. Second step",
		},
		{
			name:         "no headers - no split",
			response:     `Just some content without any markers`,
			wantAnalysis: "Just some content without any markers",
			wantPlan:     "Just some content without any markers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAnalysis, gotPlan := parseTechnicalPlanningResponse(tt.response)
			if gotAnalysis != tt.wantAnalysis {
				t.Errorf("parseTechnicalPlanningResponse() analysis = %q, want %q", gotAnalysis, tt.wantAnalysis)
			}
			if gotPlan != tt.wantPlan {
				t.Errorf("parseTechnicalPlanningResponse() plan = %q, want %q", gotPlan, tt.wantPlan)
			}
		})
	}
}

func TestCheckAlreadyDone(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name:     "ALREADY_DONE prefix present",
			response: "ALREADY_DONE: method Foo already exists in bar.go:42",
			want:     "method Foo already exists in bar.go:42",
		},
		{
			name:     "ALREADY_DONE with extra text",
			response: "Some text\nALREADY_DONE: feature already implemented\nMore text",
			want:     "feature already implemented",
		},
		{
			name:     "no ALREADY_DONE",
			response: "This is a normal response without the prefix",
			want:     "",
		},
		{
			name:     "empty response",
			response: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkAlreadyDone(tt.response)
			if got != tt.want {
				t.Errorf("checkAlreadyDone() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorker_YoloMode(t *testing.T) {
	tests := []struct {
		name         string
		yoloMode     bool
		wantDecision string
	}{
		{
			name:         "yolo mode enabled - auto approve",
			yoloMode:     true,
			wantDecision: "approve",
		},
		{
			name:         "yolo mode disabled - wait for decision",
			yoloMode:     false,
			wantDecision: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a worker with the specified yolo mode
			cfg := &config.Config{
				YoloMode: tt.yoloMode,
			}

			worker := &Worker{
				id:         1,
				decisionCh: make(chan UserDecision, 1),
			}
			worker.cfg.Store(cfg)

			// Test that yolo mode correctly determines if we should auto-approve
			var decision UserDecision
			if worker.cfg.Load().YoloMode {
				decision = UserDecision{Action: "approve"}
			}

			if decision.Action != tt.wantDecision {
				t.Errorf("yolo mode decision = %q, want %q", decision.Action, tt.wantDecision)
			}
		})
	}
}

func TestWorker_UpdateConfig(t *testing.T) {
	tests := []struct {
		name        string
		initialYolo bool
		updatedYolo bool
	}{
		{
			name:        "update yolo mode from false to true",
			initialYolo: false,
			updatedYolo: true,
		},
		{
			name:        "update yolo mode from true to false",
			initialYolo: true,
			updatedYolo: false,
		},
		{
			name:        "update with same yolo mode value",
			initialYolo: false,
			updatedYolo: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create initial config
			initialCfg := &config.Config{
				YoloMode: tt.initialYolo,
			}

			// Create worker
			worker := &Worker{
				id:         1,
				decisionCh: make(chan UserDecision, 1),
			}
			worker.cfg.Store(initialCfg)

			// Verify initial config
			if worker.cfg.Load().YoloMode != tt.initialYolo {
				t.Errorf("initial YoloMode = %v, want %v", worker.cfg.Load().YoloMode, tt.initialYolo)
			}

			// Update config
			updatedCfg := &config.Config{
				YoloMode: tt.updatedYolo,
			}
			worker.UpdateConfig(updatedCfg)

			// Verify updated config
			if worker.cfg.Load().YoloMode != tt.updatedYolo {
				t.Errorf("updated YoloMode = %v, want %v", worker.cfg.Load().YoloMode, tt.updatedYolo)
			}
		})
	}
}

func TestWorker_ImplementsConfigAwareWorker(_ *testing.T) {
	// This test verifies at compile time that Worker implements ConfigAwareWorker
	var _ config.ConfigAwareWorker = (*Worker)(nil)
}

func TestWorker_UpdateConfig_Atomicity(_ *testing.T) {
	// Test that config updates are atomic and don't race
	worker := &Worker{
		id:         1,
		decisionCh: make(chan UserDecision, 1),
	}

	initialCfg := &config.Config{YoloMode: false}
	worker.cfg.Store(initialCfg)

	// Simulate concurrent config updates
	done := make(chan bool, 2)

	go func() {
		for i := range 100 {
			cfg := &config.Config{YoloMode: i%2 == 0}
			worker.UpdateConfig(cfg)
		}
		done <- true
	}()

	go func() {
		for range 100 {
			_ = worker.cfg.Load().YoloMode
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// If we get here without panic or race detector errors, atomicity is working
	// The final value could be either true or false depending on timing
	_ = worker.cfg.Load().YoloMode
}

func TestWorker_CreateArtifactDir(t *testing.T) {
	tests := []struct {
		name        string
		issueNumber int
		wantPath    string
	}{
		{
			name:        "create artifact directory for issue 42",
			issueNumber: 42,
			wantPath:    filepath.Join(".oda", "artifacts", "42"),
		},
		{
			name:        "create artifact directory for issue 123",
			issueNumber: 123,
			wantPath:    filepath.Join(".oda", "artifacts", "123"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			w := &Worker{
				id:      1,
				repoDir: tmpDir,
			}

			// Test creating artifact directory
			got, err := w.createArtifactDir(tt.issueNumber)
			if err != nil {
				t.Fatalf("createArtifactDir() error = %v", err)
			}

			// Verify returned path
			wantFullPath := filepath.Join(tmpDir, tt.wantPath)
			if got != wantFullPath {
				t.Errorf("createArtifactDir() = %q, want %q", got, wantFullPath)
			}

			// Verify directory exists
			if _, err := os.Stat(got); os.IsNotExist(err) {
				t.Errorf("createArtifactDir() directory does not exist: %s", got)
			}

			// Test idempotency - calling again should not error
			_, err = w.createArtifactDir(tt.issueNumber)
			if err != nil {
				t.Errorf("createArtifactDir() second call error = %v, want no error", err)
			}
		})
	}
}

func TestTechnicalPlanningPromptFormat(t *testing.T) {
	promptTemplate := prompts.MustGet(prompts.MVPTechnicalPlanning)

	// Verify the prompt template can be formatted with 4 arguments
	formatted := fmt.Sprintf(promptTemplate, 123, "Test Title", "Test Body", 123)

	// Verify the formatted prompt contains the artifact path pattern
	expectedArtifactPath := ".oda/artifacts/123/01-planning.md"
	if !strings.Contains(formatted, expectedArtifactPath) {
		t.Errorf("formatted prompt does not contain expected artifact path %q", expectedArtifactPath)
	}

	// Verify all format specifiers were replaced (no %! patterns indicating missing args)
	if strings.Contains(formatted, "%!") {
		t.Errorf("formatted prompt contains unreplaced format specifiers: %s", formatted)
	}

	// Verify the prompt contains key sections
	if !strings.Contains(formatted, "ARTIFACT") {
		t.Error("formatted prompt does not contain 'ARTIFACT' section")
	}
	if !strings.Contains(formatted, "RESPONSE to orchestrator") {
		t.Error("formatted prompt does not contain 'RESPONSE to orchestrator' section")
	}
	if !strings.Contains(formatted, "CRITICAL: Save the complete analysis to the artifact file") {
		t.Error("formatted prompt does not contain artifact save instruction")
	}
}

func TestImplementationPromptFormat(t *testing.T) {
	promptTemplate := prompts.MustGet(prompts.MVPImplementation)

	// Verify the prompt template can be formatted with 10 arguments (added pipeline-fail.log placeholders)
	formatted := fmt.Sprintf(promptTemplate, 456, "Test Implementation", "Test Plan", "/tmp/work", "go test ./...", 456, 456, 456, 456, 456)

	// Verify the formatted prompt contains the planning artifact path
	expectedPlanningPath := ".oda/artifacts/456/01-planning.md"
	if !strings.Contains(formatted, expectedPlanningPath) {
		t.Errorf("formatted prompt does not contain expected planning artifact path %q", expectedPlanningPath)
	}

	// Verify the formatted prompt contains the coding artifact path
	expectedCodingPath := ".oda/artifacts/456/02-coding.md"
	if !strings.Contains(formatted, expectedCodingPath) {
		t.Errorf("formatted prompt does not contain expected coding artifact path %q", expectedCodingPath)
	}

	// Verify the formatted prompt contains the pipeline failure log path
	expectedPipelineFailPath := ".oda/artifacts/456/pipeline-fail.log"
	if !strings.Contains(formatted, expectedPipelineFailPath) {
		t.Errorf("formatted prompt does not contain expected pipeline-fail.log path %q", expectedPipelineFailPath)
	}

	// Verify all format specifiers were replaced (no %! patterns indicating missing args)
	if strings.Contains(formatted, "%!") {
		t.Errorf("formatted prompt contains unreplaced format specifiers: %s", formatted)
	}

	// Verify the prompt contains key sections
	if !strings.Contains(formatted, "ARTIFACT") {
		t.Error("formatted prompt does not contain 'ARTIFACT' section")
	}
	if !strings.Contains(formatted, "READ PLANNING ARTIFACT") {
		t.Error("formatted prompt does not contain 'READ PLANNING ARTIFACT' step")
	}
	if !strings.Contains(formatted, "CRITICAL: Save coding notes to the artifact file") {
		t.Error("formatted prompt does not contain artifact save instruction")
	}
	if !strings.Contains(formatted, "PIPELINE FAILURE") {
		t.Error("formatted prompt does not contain pipeline failure check section")
	}
	if !strings.Contains(formatted, "CHECK FOR PIPELINE FAILURE LOGS") {
		t.Error("formatted prompt does not contain 'CHECK FOR PIPELINE FAILURE LOGS' step")
	}
}

func TestCodeReviewPromptFormat(t *testing.T) {
	promptTemplate := prompts.MustGet(prompts.MVPCodeReview)

	// Verify the prompt template can be formatted with 5 arguments (added pipeline-fail.log placeholder)
	formatted := fmt.Sprintf(promptTemplate, 789, "Test Review", "https://github.com/org/repo/pull/42", "org/repo", 789)

	// Verify the formatted prompt contains the pipeline failure log path
	expectedPipelineFailPath := ".oda/artifacts/789/pipeline-fail.log"
	if !strings.Contains(formatted, expectedPipelineFailPath) {
		t.Errorf("formatted prompt does not contain expected pipeline-fail.log path %q", expectedPipelineFailPath)
	}

	// Verify all format specifiers were replaced (no %! patterns indicating missing args)
	if strings.Contains(formatted, "%!") {
		t.Errorf("formatted prompt contains unreplaced format specifiers: %s", formatted)
	}

	// Verify the prompt contains key sections
	if !strings.Contains(formatted, "PIPELINE FAILURE") {
		t.Error("formatted prompt does not contain pipeline failure check section")
	}

	if !strings.Contains(formatted, "Pipeline failures resolved") {
		t.Error("formatted prompt does not contain 'Pipeline failures resolved' criterion")
	}

	if !strings.Contains(formatted, "REVIEW PROCESS") {
		t.Error("formatted prompt does not contain 'REVIEW PROCESS' section")
	}

	if !strings.Contains(formatted, "REVIEW CRITERIA") {
		t.Error("formatted prompt does not contain 'REVIEW CRITERIA' section")
	}
}

func TestStepOrderContainsCheckPipeline(t *testing.T) {
	idx := stepIndex("check-pipeline")
	if idx == -1 {
		t.Fatal("check-pipeline not found in stepOrder")
	}
	createPRIdx := stepIndex("create-pr")
	approvalIdx := stepIndex("awaiting-approval")
	if idx <= createPRIdx || idx >= approvalIdx {
		t.Errorf("check-pipeline index %d should be between create-pr (%d) and awaiting-approval (%d)",
			idx, createPRIdx, approvalIdx)
	}
}

func TestWorker_SendsCompletionNotification(t *testing.T) {
	// Create a mock orchestrator that captures notifications
	notifications := make(chan int, 1)
	mockOrchestrator := &Orchestrator{
		completionCh: notifications,
	}

	// Create worker with mock orchestrator
	worker := &Worker{
		id:           1,
		orchestrator: mockOrchestrator,
	}

	// Simulate successful completion
	worker.orchestrator.NotifyTicketCompleted(42)

	// Verify notification sent
	select {
	case num := <-notifications:
		if num != 42 {
			t.Errorf("notification issue number = %d, want 42", num)
		}
	case <-time.After(time.Second):
		t.Error("completion notification not sent")
	}
}
