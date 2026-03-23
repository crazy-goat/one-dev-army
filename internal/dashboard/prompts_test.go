package dashboard

import (
	"strings"
	"testing"
)

// TestRefinementPrompt_EnglishOnlyConstraint verifies prompt requires English output
func TestRefinementPrompt_EnglishOnlyConstraint(t *testing.T) {
	if !strings.Contains(RefinementPromptTemplate, "Output MUST be in English regardless of input language") {
		t.Error("RefinementPromptTemplate must contain English-only constraint")
	}
}
