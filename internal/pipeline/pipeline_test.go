package pipeline_test

import (
	"errors"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/pipeline"
)

func TestStageProgression(t *testing.T) {
	expected := []pipeline.Stage{
		pipeline.StageQueued,
		pipeline.StageAnalysis,
		pipeline.StageCoding,
		pipeline.StageCodeReview,
		pipeline.StageCreatePR,
		pipeline.StageCheckPipeline,
		pipeline.StageApprove,
		pipeline.StageMerging,
		pipeline.StageDone,
	}

	stage := pipeline.StageQueued
	for i := range expected {
		if stage != expected[i] {
			t.Fatalf("step %d: got %q, want %q", i, stage, expected[i])
		}
		if stage == pipeline.StageDone {
			break
		}
		stage = stage.Next()
	}
}

func TestStageColumns(t *testing.T) {
	tests := []struct {
		stage pipeline.Stage
		want  pipeline.Column
	}{
		{pipeline.StageQueued, pipeline.ColumnBacklog},
		{pipeline.StageAnalysis, pipeline.ColumnPlan},
		{pipeline.StageCoding, pipeline.ColumnCode},
		{pipeline.StageCodeReview, pipeline.ColumnAIReview},
		{pipeline.StageCreatePR, pipeline.ColumnAIReview},
		{pipeline.StageCheckPipeline, pipeline.ColumnCheckPipeline},
		{pipeline.StageApprove, pipeline.ColumnApprove},
		{pipeline.StageMerging, pipeline.ColumnMerge},
		{pipeline.StageDone, pipeline.ColumnDone},
		{pipeline.StageFailed, pipeline.ColumnFailed},
		{pipeline.StageBlocked, pipeline.ColumnBlocked},
	}

	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			got := tt.stage.Column()
			if got != tt.want {
				t.Errorf("Stage(%q).Column() = %q, want %q", tt.stage, got, tt.want)
			}
		})
	}
}

func TestStageLabels(t *testing.T) {
	tests := []struct {
		stage pipeline.Stage
		want  string
	}{
		{pipeline.StageQueued, ""},
		{pipeline.StageAnalysis, "stage:analysis"},
		{pipeline.StageCoding, "stage:coding"},
		{pipeline.StageCodeReview, "stage:code-review"},
		{pipeline.StageCreatePR, "stage:create-pr"},
		{pipeline.StageCheckPipeline, "stage:check-pipeline"},
		{pipeline.StageApprove, "stage:awaiting-approval"},
		{pipeline.StageMerging, "stage:merging"},
		{pipeline.StageFailed, "stage:failed"},
		{pipeline.StageBlocked, "stage:blocked"},
		{pipeline.StageDone, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			got := tt.stage.Label()
			if got != tt.want {
				t.Errorf("Stage(%q).Label() = %q, want %q", tt.stage, got, tt.want)
			}
		})
	}
}

func TestRetryTarget(t *testing.T) {
	tests := []struct {
		stage pipeline.Stage
		want  pipeline.Stage
	}{
		{pipeline.StageQueued, pipeline.StageQueued},
		{pipeline.StageAnalysis, pipeline.StageAnalysis},
		{pipeline.StageCoding, pipeline.StageCoding},
		{pipeline.StageCodeReview, pipeline.StageCoding},
		{pipeline.StageCreatePR, pipeline.StageCoding},
		{pipeline.StageCheckPipeline, pipeline.StageCoding},
		{pipeline.StageMerging, pipeline.StageCoding},
	}

	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			got := tt.stage.RetryTarget()
			if got != tt.want {
				t.Errorf("Stage(%q).RetryTarget() = %q, want %q", tt.stage, got, tt.want)
			}
		})
	}
}

type mockExecutor struct {
	fn func(taskID int, stage pipeline.Stage, context string) (*pipeline.StageResult, error)
}

func (m *mockExecutor) Execute(taskID int, stage pipeline.Stage, context string) (*pipeline.StageResult, error) {
	return m.fn(taskID, stage, context)
}

func TestPipelineSuccess(t *testing.T) {
	exec := &mockExecutor{
		fn: func(_ int, stage pipeline.Stage, _ string) (*pipeline.StageResult, error) {
			return &pipeline.StageResult{Stage: stage, Success: true, Output: "ok"}, nil
		},
	}

	var changes []pipeline.Stage
	p := pipeline.New(3, exec, func(_ int, s pipeline.Stage) {
		changes = append(changes, s)
	})

	result, err := p.Run(1, pipeline.StageAnalysis, "test context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stage != pipeline.StageDone {
		t.Errorf("final stage = %q, want %q", result.Stage, pipeline.StageDone)
	}

	expectedStages := []pipeline.Stage{
		pipeline.StageAnalysis,
		pipeline.StageCoding,
		pipeline.StageCodeReview,
		pipeline.StageCreatePR,
		pipeline.StageCheckPipeline,
		pipeline.StageApprove,
		pipeline.StageMerging,
		pipeline.StageDone,
	}
	if len(changes) != len(expectedStages) {
		t.Fatalf("got %d stage changes, want %d: %v", len(changes), len(expectedStages), changes)
	}
	for i, want := range expectedStages {
		if changes[i] != want {
			t.Errorf("change[%d] = %q, want %q", i, changes[i], want)
		}
	}
}

func TestPipelineRetryAndBlock(t *testing.T) {
	exec := &mockExecutor{
		fn: func(_ int, stage pipeline.Stage, _ string) (*pipeline.StageResult, error) {
			if stage == pipeline.StageCodeReview {
				return &pipeline.StageResult{Stage: stage, Success: false, Output: "review failed"}, nil
			}
			return &pipeline.StageResult{Stage: stage, Success: true, Output: "ok"}, nil
		},
	}

	var changes []pipeline.Stage
	p := pipeline.New(3, exec, func(_ int, s pipeline.Stage) {
		changes = append(changes, s)
	})

	result, err := p.Run(1, pipeline.StageAnalysis, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stage != pipeline.StageBlocked {
		t.Errorf("final stage = %q, want %q", result.Stage, pipeline.StageBlocked)
	}

	expectedChanges := []pipeline.Stage{
		pipeline.StageAnalysis,
		pipeline.StageCoding,
		pipeline.StageCodeReview,
		pipeline.StageCoding,
		pipeline.StageCodeReview,
		pipeline.StageCoding,
		pipeline.StageCodeReview,
		pipeline.StageBlocked,
	}
	if len(changes) != len(expectedChanges) {
		t.Fatalf("got %d stage changes, want %d: %v", len(changes), len(expectedChanges), changes)
	}
	for i, want := range expectedChanges {
		if changes[i] != want {
			t.Errorf("change[%d] = %q, want %q", i, changes[i], want)
		}
	}
}

func TestPipelineRetryThenSucceed(t *testing.T) {
	callCount := 0
	exec := &mockExecutor{
		fn: func(_ int, stage pipeline.Stage, _ string) (*pipeline.StageResult, error) {
			if stage == pipeline.StageCodeReview {
				callCount++
				if callCount <= 2 {
					return &pipeline.StageResult{Stage: stage, Success: false, Output: "review failed"}, nil
				}
			}
			return &pipeline.StageResult{Stage: stage, Success: true, Output: "ok"}, nil
		},
	}

	p := pipeline.New(5, exec, nil)

	result, err := p.Run(1, pipeline.StageCoding, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stage != pipeline.StageDone {
		t.Errorf("final stage = %q, want %q", result.Stage, pipeline.StageDone)
	}
}

func TestPipelineContextPropagation(t *testing.T) {
	contexts := make(map[pipeline.Stage]string)
	exec := &mockExecutor{
		fn: func(_ int, stage pipeline.Stage, ctx string) (*pipeline.StageResult, error) {
			contexts[stage] = ctx
			return &pipeline.StageResult{
				Stage:   stage,
				Success: true,
				Output:  "output-from-" + string(stage),
			}, nil
		},
	}

	p := pipeline.New(3, exec, nil)
	_, err := p.Run(1, pipeline.StageAnalysis, "initial-context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Analysis should receive the initial context
	if contexts[pipeline.StageAnalysis] != "initial-context" {
		t.Errorf("analysis context = %q, want %q", contexts[pipeline.StageAnalysis], "initial-context")
	}

	// Coding should receive the output from analysis
	if contexts[pipeline.StageCoding] != "output-from-analysis" {
		t.Errorf("coding context = %q, want %q", contexts[pipeline.StageCoding], "output-from-analysis")
	}

	// Code review should receive the output from coding
	if contexts[pipeline.StageCodeReview] != "output-from-coding" {
		t.Errorf("code-review context = %q, want %q", contexts[pipeline.StageCodeReview], "output-from-coding")
	}
}

func TestPipelineExecutorError(t *testing.T) {
	exec := &mockExecutor{
		fn: func(_ int, _ pipeline.Stage, _ string) (*pipeline.StageResult, error) {
			return nil, errors.New("executor crashed")
		},
	}

	p := pipeline.New(3, exec, nil)

	_, err := p.Run(1, pipeline.StageAnalysis, "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
