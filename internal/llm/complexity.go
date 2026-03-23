package llm

import (
	"regexp"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
)

// DetectComplexity analyzes content and returns an appropriate complexity level
func DetectComplexity(content string) config.ComplexityLevel {
	if content == "" {
		return config.ComplexityMedium
	}

	score := calculateComplexityScore(content)

	// Map score to complexity level
	if score <= 30 {
		return config.ComplexityLow
	} else if score <= 70 {
		return config.ComplexityMedium
	}
	return config.ComplexityHigh
}

// calculateComplexityScore returns a score from 0-100 based on content analysis
func calculateComplexityScore(content string) int {
	score := 0
	contentLower := strings.ToLower(content)

	// Factor 1: Content length (up to 20 points)
	length := len(content)
	if length > 5000 {
		score += 20
	} else if length > 2000 {
		score += 15
	} else if length > 1000 {
		score += 10
	} else if length > 500 {
		score += 5
	}

	// Factor 2: Number of files mentioned (up to 20 points)
	fileCount := countFileReferences(content)
	if fileCount > 20 {
		score += 20
	} else if fileCount > 10 {
		score += 15
	} else if fileCount > 5 {
		score += 10
	} else if fileCount > 2 {
		score += 5
	}

	// Factor 3: Complexity keywords (up to 25 points)
	complexityKeywords := []string{
		"architecture", "refactor", "redesign", "restructure",
		"distributed", "concurrency", "parallel", "async",
		"microservice", "scalability", "performance", "optimization",
		"security", "authentication", "authorization", "encryption",
		"database", "migration", "schema", "transaction",
		"api", "integration", "webhook", "protocol",
		"algorithm", "data structure", "complex logic",
		"machine learning", "ai", "model", "training",
		"kubernetes", "docker", "container", "orchestration",
		"ci/cd", "pipeline", "deployment", "infrastructure",
	}

	keywordCount := 0
	for _, keyword := range complexityKeywords {
		if strings.Contains(contentLower, keyword) {
			keywordCount++
		}
	}

	if keywordCount >= 8 {
		score += 25
	} else if keywordCount >= 5 {
		score += 20
	} else if keywordCount >= 3 {
		score += 15
	} else if keywordCount >= 1 {
		score += 5
	}

	// Factor 4: Code blocks and technical depth (up to 15 points)
	codeBlockCount := countCodeBlocks(content)
	if codeBlockCount > 5 {
		score += 15
	} else if codeBlockCount > 3 {
		score += 10
	} else if codeBlockCount > 1 {
		score += 5
	}

	// Factor 5: Dependencies and requirements (up to 10 points)
	depKeywords := []string{"dependency", "depends on", "requires", "prerequisite", "blocked by"}
	depCount := 0
	for _, keyword := range depKeywords {
		depCount += strings.Count(contentLower, keyword)
	}
	if depCount >= 5 {
		score += 10
	} else if depCount >= 3 {
		score += 7
	} else if depCount >= 1 {
		score += 3
	}

	// Factor 6: Testing requirements (up to 10 points)
	testKeywords := []string{"test", "testing", "e2e", "integration test", "unit test", "coverage"}
	testCount := 0
	for _, keyword := range testKeywords {
		testCount += strings.Count(contentLower, keyword)
	}
	if testCount >= 5 {
		score += 10
	} else if testCount >= 3 {
		score += 7
	} else if testCount >= 1 {
		score += 3
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

// countFileReferences counts the number of file references in content
func countFileReferences(content string) int {
	// Match patterns like:
	// - `filename.go`
	// - "filename.go"
	// - filename.go (at word boundary)
	// - /path/to/file.go
	patterns := []*regexp.Regexp{
		regexp.MustCompile("`[^`]+\\.[^`]+`"),                   // Backtick quoted files
		regexp.MustCompile("\"[^\"]+\\.[^\"]+\""),               // Double quoted files
		regexp.MustCompile("\\b[A-Za-z0-9_/-]+\\.[a-zA-Z]+\\b"), // File paths with extensions
	}

	seen := make(map[string]bool)
	for _, pattern := range patterns {
		matches := pattern.FindAllString(content, -1)
		for _, match := range matches {
			// Clean up the match
			clean := strings.Trim(match, "`\"'")
			if len(clean) > 2 && strings.Contains(clean, ".") {
				seen[clean] = true
			}
		}
	}

	return len(seen)
}

// countCodeBlocks counts the number of code blocks in markdown content
func countCodeBlocks(content string) int {
	// Match markdown code blocks ```...```
	codeBlockPattern := regexp.MustCompile("(?s)```[^`]*```")
	return len(codeBlockPattern.FindAllString(content, -1))
}

// DetectComplexityWithHints detects complexity with additional hints
func DetectComplexityWithHints(content string, hints map[string]interface{}) config.ComplexityLevel {
	// Check for explicit complexity hint
	if hints != nil {
		if explicit, ok := hints["complexity"].(string); ok {
			switch config.ComplexityLevel(explicit) {
			case config.ComplexityLow, config.ComplexityMedium, config.ComplexityHigh:
				return config.ComplexityLevel(explicit)
			}
		}

		// Check for size label hint
		if sizeLabel, ok := hints["size_label"].(string); ok {
			switch sizeLabel {
			case "size:S":
				return config.ComplexityLow
			case "size:M":
				return config.ComplexityMedium
			case "size:L", "size:XL":
				return config.ComplexityHigh
			}
		}

		// Check for file count hint
		if fileCount, ok := hints["file_count"].(int); ok {
			if fileCount > 15 {
				return config.ComplexityHigh
			} else if fileCount > 5 {
				return config.ComplexityMedium
			}
			return config.ComplexityLow
		}
	}

	// Fall back to content analysis
	return DetectComplexity(content)
}

// ComplexityAnalyzer provides more sophisticated complexity analysis
type ComplexityAnalyzer struct {
	// Weights for different factors (can be customized)
	Weights ComplexityWeights
}

// ComplexityWeights holds the weighting factors for complexity calculation
type ComplexityWeights struct {
	LengthWeight     int
	FileCountWeight  int
	KeywordWeight    int
	CodeBlockWeight  int
	DependencyWeight int
	TestingWeight    int
}

// DefaultWeights returns the default complexity weights
func DefaultWeights() ComplexityWeights {
	return ComplexityWeights{
		LengthWeight:     20,
		FileCountWeight:  20,
		KeywordWeight:    25,
		CodeBlockWeight:  15,
		DependencyWeight: 10,
		TestingWeight:    10,
	}
}

// NewComplexityAnalyzer creates a new analyzer with default weights
func NewComplexityAnalyzer() *ComplexityAnalyzer {
	return &ComplexityAnalyzer{
		Weights: DefaultWeights(),
	}
}

// Analyze performs a weighted complexity analysis
func (a *ComplexityAnalyzer) Analyze(content string) config.ComplexityLevel {
	if content == "" {
		return config.ComplexityMedium
	}

	score := a.calculateWeightedScore(content)

	// Map score to complexity level
	if score <= 30 {
		return config.ComplexityLow
	} else if score <= 70 {
		return config.ComplexityMedium
	}
	return config.ComplexityHigh
}

func (a *ComplexityAnalyzer) calculateWeightedScore(content string) int {
	score := 0
	contentLower := strings.ToLower(content)
	w := a.Weights

	// Length score
	length := len(content)
	maxLengthScore := float64(w.LengthWeight)
	if length > 5000 {
		score += w.LengthWeight
	} else {
		score += int(float64(length) / 5000.0 * maxLengthScore)
	}

	// File count score
	fileCount := countFileReferences(content)
	if fileCount > 20 {
		score += w.FileCountWeight
	} else {
		score += int(float64(fileCount) / 20.0 * float64(w.FileCountWeight))
	}

	// Keyword score
	complexityKeywords := []string{
		"architecture", "refactor", "redesign", "restructure",
		"distributed", "concurrency", "parallel", "async",
		"microservice", "scalability", "performance", "optimization",
		"security", "authentication", "authorization", "encryption",
		"database", "migration", "schema", "transaction",
		"api", "integration", "webhook", "protocol",
		"algorithm", "data structure", "complex logic",
		"machine learning", "ai", "model", "training",
		"kubernetes", "docker", "container", "orchestration",
		"ci/cd", "pipeline", "deployment", "infrastructure",
	}

	keywordCount := 0
	for _, keyword := range complexityKeywords {
		if strings.Contains(contentLower, keyword) {
			keywordCount++
		}
	}

	if keywordCount >= 10 {
		score += w.KeywordWeight
	} else {
		score += int(float64(keywordCount) / 10.0 * float64(w.KeywordWeight))
	}

	// Code block score
	codeBlockCount := countCodeBlocks(content)
	if codeBlockCount > 5 {
		score += w.CodeBlockWeight
	} else {
		score += int(float64(codeBlockCount) / 5.0 * float64(w.CodeBlockWeight))
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}
