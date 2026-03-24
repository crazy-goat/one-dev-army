package llm_test

import (
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/llm"
)

func TestRouter_SelectModel(t *testing.T) {
	cfg := config.DefaultLLMConfig()
	router := llm.NewRouter(&cfg)

	tests := []struct {
		name         string
		category     config.TaskCategory
		complexity   config.ComplexityLevel
		wantNonEmpty bool
	}{
		{
			name:         "code low complexity",
			category:     config.CategoryCode,
			complexity:   config.ComplexityLow,
			wantNonEmpty: true,
		},
		{
			name:         "code high complexity",
			category:     config.CategoryCode,
			complexity:   config.ComplexityHigh,
			wantNonEmpty: true,
		},
		{
			name:         "planning medium complexity",
			category:     config.CategoryPlanning,
			complexity:   config.ComplexityMedium,
			wantNonEmpty: true,
		},
		{
			name:         "orchestration low complexity",
			category:     config.CategoryOrchestration,
			complexity:   config.ComplexityLow,
			wantNonEmpty: true,
		},
		{
			name:         "setup medium complexity",
			category:     config.CategorySetup,
			complexity:   config.ComplexityMedium,
			wantNonEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.SelectModel(tt.category, tt.complexity, nil)
			if tt.wantNonEmpty && got == "" {
				t.Errorf("SelectModel() = %q, want non-empty", got)
			}
		})
	}
}

func TestRouter_SelectModelForStage(t *testing.T) {
	cfg := config.DefaultLLMConfig()
	router := llm.NewRouter(&cfg)

	tests := []struct {
		name         string
		stage        string
		context      string
		wantNonEmpty bool
	}{
		{
			name:         "analysis stage",
			stage:        "analysis",
			context:      "test context",
			wantNonEmpty: true,
		},
		{
			name:         "coding stage",
			stage:        "coding",
			context:      "test context",
			wantNonEmpty: true,
		},
		{
			name:         "code-review stage",
			stage:        "code-review",
			context:      "test context",
			wantNonEmpty: true,
		},
		{
			name:         "plan-review stage (force strong)",
			stage:        "plan-review",
			context:      "test context",
			wantNonEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.SelectModelForStage(tt.stage, tt.context)
			if tt.wantNonEmpty && got == "" {
				t.Errorf("SelectModelForStage() = %q, want non-empty", got)
			}
		})
	}
}

func TestRouter_UpdateConfig(t *testing.T) {
	cfg := config.DefaultLLMConfig()
	router := llm.NewRouter(&cfg)

	// Test that config can be updated
	newCfg := config.DefaultLLMConfig()
	newCfg.Code.Model = "new-code-model"

	router.UpdateConfig(&newCfg)

	// Verify the update
	updatedCfg := router.GetConfig()
	if updatedCfg.Code.Model != "new-code-model" {
		t.Errorf("UpdateConfig() failed, model = %q, want %q", updatedCfg.Code.Model, "new-code-model")
	}
}

func TestRouter_OnReload(t *testing.T) {
	cfg := config.DefaultLLMConfig()
	router := llm.NewRouter(&cfg)

	reloadCalled := false
	router.OnReload(func() {
		reloadCalled = true
	})

	// Update config to trigger reload callbacks synchronously
	newCfg := config.DefaultLLMConfig()
	router.UpdateConfig(&newCfg)

	if !reloadCalled {
		t.Error("OnReload callback was not invoked after UpdateConfig")
	}
}

func TestComplexityAnalyzer(t *testing.T) {
	thresholds := llm.ComplexityThresholds{
		CodeSizeThreshold:       100,
		HighComplexityThreshold: 500,
		FileCountThreshold:      5,
	}

	analyzer := llm.NewComplexityAnalyzer(thresholds)

	t.Run("analyze simple task", func(t *testing.T) {
		report := analyzer.AnalyzeTask("Fix typo", []string{}, nil)
		if report.Complexity != config.ComplexityLow {
			t.Errorf("expected low complexity, got %v", report.Complexity)
		}
	})

	t.Run("analyze complex task", func(t *testing.T) {
		report := analyzer.AnalyzeTask(
			"Implement distributed microservices architecture with database migration",
			[]string{"service.go", "db.go", "api.go", "config.go", "main.go", "docker-compose.yml"},
			nil,
		)
		if report.Complexity != config.ComplexityHigh {
			t.Errorf("expected high complexity, got %v", report.Complexity)
		}
	})
}

func TestRoutingHints(t *testing.T) {
	hints := llm.NewRoutingHints().
		WithStage("code-review").
		WithFileCount(10).
		WithCodeSize(1000).
		WithPriority("high").
		Build()

	if hints["stage"] != "code-review" {
		t.Errorf("stage hint = %v, want code-review", hints["stage"])
	}
	if hints["file_count"] != 10 {
		t.Errorf("file_count hint = %v, want 10", hints["file_count"])
	}
	if hints["code_size"] != 1000 {
		t.Errorf("code_size hint = %v, want 1000", hints["code_size"])
	}
	if hints["priority"] != "high" {
		t.Errorf("priority hint = %v, want high", hints["priority"])
	}
}

func TestEstimateFromKeywords(t *testing.T) {
	keywords := llm.DefaultTaskKeywords()

	tests := []struct {
		text     string
		expected config.ComplexityLevel
	}{
		{
			text:     "Fix typo in documentation",
			expected: config.ComplexityLow,
		},
		{
			text:     "Implement user authentication feature",
			expected: config.ComplexityHigh, // "authentication" is a high complexity keyword (security), giving 3 points
		},
		{
			text:     "Refactor database layer with microservices",
			expected: config.ComplexityHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := llm.EstimateFromKeywords(tt.text, keywords)
			if got != tt.expected {
				t.Errorf("EstimateFromKeywords() = %v, want %v", got, tt.expected)
			}
		})
	}
}
