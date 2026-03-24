package llm

import (
	"regexp"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
)

// countLines counts non-empty lines in text
func countLines(text string) int {
	if text == "" {
		return 0
	}

	lines := strings.Split(text, "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// complexityIndicators are patterns that suggest high complexity
var complexityIndicators = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(refactor|rearchitecture|redesign)\b`),
	regexp.MustCompile(`(?i)\b(algorithm|complex|optimization)\b`),
	regexp.MustCompile(`(?i)\b(concurrency|parallel|async|goroutine|thread)\b`),
	regexp.MustCompile(`(?i)\b(distributed|microservice|service mesh)\b`),
	regexp.MustCompile(`(?i)\b(critical|security|performance|scalability)\b`),
	regexp.MustCompile(`(?i)\b(database migration|schema change)\b`),
	regexp.MustCompile(`(?i)\b(api design|interface design|protocol)\b`),
}

// countComplexityIndicators counts how many high-complexity patterns are found
func countComplexityIndicators(text string) int {
	score := 0
	for _, re := range complexityIndicators {
		if re.MatchString(text) {
			score++
		}
	}
	return score
}

// ComplexityAnalyzer provides detailed complexity analysis for tasks
type ComplexityAnalyzer struct {
	thresholds config.ComplexityThresholds
}

// NewComplexityAnalyzer creates a new complexity analyzer
func NewComplexityAnalyzer(thresholds config.ComplexityThresholds) *ComplexityAnalyzer {
	if thresholds.CodeSizeThreshold == 0 {
		thresholds = config.DefaultLLMConfig().RoutingRules.ComplexityThresholds
	}

	return &ComplexityAnalyzer{
		thresholds: thresholds,
	}
}

// AnalyzeTask performs a comprehensive complexity analysis
func (ca *ComplexityAnalyzer) AnalyzeTask(description string, files []string, context map[string]any) ComplexityReport {
	report := ComplexityReport{
		Description: description,
		Files:       files,
	}

	// Calculate base metrics
	report.LineCount = countLines(description)
	report.FileCount = len(files)
	report.IndicatorScore = countComplexityIndicators(description)

	// Check for additional context
	if codeSize, ok := context["code_size"].(int); ok {
		report.LineCount = codeSize
	}

	if fileCount, ok := context["file_count"].(int); ok {
		report.FileCount = fileCount
	}

	// Determine complexity level
	report.Complexity = ca.calculateComplexity(report)

	// Generate explanation
	report.Explanation = ca.generateExplanation(report)

	return report
}

// calculateComplexity determines the complexity level based on metrics
func (ca *ComplexityAnalyzer) calculateComplexity(report ComplexityReport) config.ComplexityLevel {
	score := 0

	// Line count scoring
	if report.LineCount > ca.thresholds.HighComplexityThreshold {
		score += 3
	} else if report.LineCount > ca.thresholds.CodeSizeThreshold {
		score += 1
	}

	// File count scoring
	if report.FileCount > ca.thresholds.FileCountThreshold {
		score += 2
	} else if report.FileCount > 2 {
		score += 1
	}

	// Complexity indicators
	score += report.IndicatorScore

	// Determine level
	if score >= 4 {
		return config.ComplexityHigh
	} else if score >= 2 {
		return config.ComplexityMedium
	}

	return config.ComplexityLow
}

// generateExplanation creates a human-readable explanation of the complexity assessment
func (ca *ComplexityAnalyzer) generateExplanation(report ComplexityReport) string {
	var parts []string

	if report.LineCount > ca.thresholds.HighComplexityThreshold {
		parts = append(parts, "large codebase")
	} else if report.LineCount > ca.thresholds.CodeSizeThreshold {
		parts = append(parts, "moderate code size")
	}

	if report.FileCount > ca.thresholds.FileCountThreshold {
		parts = append(parts, "many files affected")
	} else if report.FileCount > 2 {
		parts = append(parts, "multiple files")
	}

	if report.IndicatorScore > 2 {
		parts = append(parts, "complex patterns detected")
	} else if report.IndicatorScore > 0 {
		parts = append(parts, "some complexity indicators")
	}

	if len(parts) == 0 {
		return "Simple task with minimal complexity"
	}

	return "Task has " + strings.Join(parts, ", ")
}

// ComplexityReport contains the results of complexity analysis
type ComplexityReport struct {
	Description    string
	Files          []string
	LineCount      int
	FileCount      int
	IndicatorScore int
	Complexity     config.ComplexityLevel
	Explanation    string
}

// IsHighComplexity returns true if the task is high complexity
func (cr *ComplexityReport) IsHighComplexity() bool {
	return cr.Complexity == config.ComplexityHigh
}

// IsMediumComplexity returns true if the task is medium complexity
func (cr *ComplexityReport) IsMediumComplexity() bool {
	return cr.Complexity == config.ComplexityMedium
}

// IsLowComplexity returns true if the task is low complexity
func (cr *ComplexityReport) IsLowComplexity() bool {
	return cr.Complexity == config.ComplexityLow
}

// TaskKeywords defines keywords that indicate task types and complexity
type TaskKeywords struct {
	HighComplexity   []string
	MediumComplexity []string
	LowComplexity    []string
}

// DefaultTaskKeywords returns the default set of task keywords
func DefaultTaskKeywords() TaskKeywords {
	return TaskKeywords{
		HighComplexity: []string{
			"refactor", "rearchitecture", "redesign",
			"algorithm", "complex", "optimization",
			"concurrency", "parallel", "distributed",
			"microservice", "service mesh",
			"security", "authentication", "authorization",
			"performance", "scalability", "high availability",
			"database migration", "schema change",
			"api design", "protocol design",
		},
		MediumComplexity: []string{
			"implement", "feature", "enhancement",
			"integration", "api", "endpoint",
			"validation", "error handling",
			"test", "testing", "coverage",
			"configuration", "setup",
		},
		LowComplexity: []string{
			"fix", "bugfix", "typo",
			"documentation", "comment",
			"logging", "metrics",
			"simple", "minor", "trivial",
		},
	}
}

// AnalyzeKeywords scans text for complexity keywords
func AnalyzeKeywords(text string, keywords TaskKeywords) (high, medium, low int) {
	textLower := strings.ToLower(text)

	for _, kw := range keywords.HighComplexity {
		if strings.Contains(textLower, kw) {
			high++
		}
	}

	for _, kw := range keywords.MediumComplexity {
		if strings.Contains(textLower, kw) {
			medium++
		}
	}

	for _, kw := range keywords.LowComplexity {
		if strings.Contains(textLower, kw) {
			low++
		}
	}

	return
}

// EstimateFromKeywords estimates complexity based on keyword analysis
func EstimateFromKeywords(text string, keywords TaskKeywords) config.ComplexityLevel {
	high, medium, low := AnalyzeKeywords(text, keywords)

	// Weight the keywords
	score := high*3 + medium*1 - low*1

	if score >= 3 {
		return config.ComplexityHigh
	} else if score >= 1 {
		return config.ComplexityMedium
	}

	return config.ComplexityLow
}

// DetectComplexity analyzes context and returns complexity level
// This is a simplified version that uses default thresholds
// Deprecated: Complexity-based routing is being phased out
func DetectComplexity(context string) config.ComplexityLevel {
	thresholds := config.DefaultLLMConfig().RoutingRules.ComplexityThresholds
	analyzer := NewComplexityAnalyzer(thresholds)
	report := analyzer.AnalyzeTask(context, []string{}, nil)
	return report.Complexity
}
