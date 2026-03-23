package pipeline_test

import (
	"fmt"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/pipeline"
)

func TestStageProgression(t *testing.T) {
	expected := []pipeline.Stage{
		pipeline.StageQueued,
		pipeline.StageAnalysis,
		pipeline.StagePlanning,
		pipeline.StagePlanReview,
		pipeline.StageCoding,
		pipeline.StageTesting,
		pipeline.StageCodeReview,
		pipeline.StageMerging,
		pipeline.StageDone,
	}

	stage := pipeline.StageQueued
	for i := 0; i < len(expected); i++ {
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
		{pipeline.StagePlanning, pipeline.ColumnPlan},
		{pipeline.StagePlanReview, pipeline.ColumnAIReview},
		{pipeline.StageCoding, pipeline.ColumnCode},
		{pipeline.StageTesting, pipeline.ColumnCode},
		{pipeline.StageCodeReview, pipeline.ColumnAIReview},
		{pipeline.StageMerging, pipeline.ColumnApprove},
		{pipeline.StageDone, pipeline.ColumnDone},
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
		{pipeline.StagePlanning, "stage:planning"},
		{pipeline.StagePlanReview, "stage:plan-review"},
		{pipeline.StageCoding, "stage:coding"},
		{pipeline.StageTesting, "stage:testing"},
		{pipeline.StageCodeReview, "stage:code-review"},
		{pipeline.StageMerging, ""},
		{pipeline.StageDone, ""},
		{pipeline.StageBlocked, ""},
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
		{pipeline.StagePlanning, pipeline.StagePlanning},
		{pipeline.StagePlanReview, pipeline.StagePlanning},
		{pipeline.StageCoding, pipeline.StageCoding},
		{pipeline.StageTesting, pipeline.StageCoding},
		{pipeline.StageCodeReview, pipeline.StageCoding},
		{pipeline.StageMerging, pipeline.StageMerging},
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
	var stages []pipeline.Stage
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
		pipeline.StagePlanning,
		pipeline.StagePlanReview,
		pipeline.StageCoding,
		pipeline.StageTesting,
		pipeline.StageCodeReview,
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

	_ = stages
}

func TestPipelineRetryAndBlock(t *testing.T) {
	exec := &mockExecutor{
		fn: func(_ int, stage pipeline.Stage, _ string) (*pipeline.StageResult, error) {
			if stage == pipeline.StagePlanReview {
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
		pipeline.StagePlanning,
		pipeline.StagePlanReview,
		pipeline.StagePlanning,
		pipeline.StagePlanReview,
		pipeline.StagePlanning,
		pipeline.StagePlanReview,
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
			if stage == pipeline.StageTesting {
				callCount++
				if callCount <= 2 {
					return &pipeline.StageResult{Stage: stage, Success: false, Output: "test failed"}, nil
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

func TestPipelineExecutorError(t *testing.T) {
	exec := &mockExecutor{
		fn: func(_ int, stage pipeline.Stage, _ string) (*pipeline.StageResult, error) {
			return nil, fmt.Errorf("executor crashed")
		},
	}

	p := pipeline.New(3, exec, nil)

	_, err := p.Run(1, pipeline.StageAnalysis, "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
