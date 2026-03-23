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

func TestBuildTechnicalPlanningPrompt(t *testing.T) {
	prompt := BuildTechnicalPlanningPrompt(WizardTypeFeature, "Add user authentication", "Go web service")

	// Verify prompt contains required sections
	if !strings.Contains(prompt, "Architecture Overview") {
		t.Error("Prompt missing Architecture Overview section")
	}
	if !strings.Contains(prompt, "Files Requiring Changes") {
		t.Error("Prompt missing Files Requiring Changes section")
	}
	if !strings.Contains(prompt, "Component Dependencies") {
		t.Error("Prompt missing Component Dependencies section")
	}
	if !strings.Contains(prompt, "Implementation Boundaries") {
		t.Error("Prompt missing Implementation Boundaries section")
	}
	if !strings.Contains(prompt, "Add user authentication") {
		t.Error("Prompt missing original idea")
	}
}

func TestBuildTechnicalPlanningPrompt_Bug(t *testing.T) {
	prompt := BuildTechnicalPlanningPrompt(WizardTypeBug, "Fix login error", "Go web service")

	if !strings.Contains(prompt, "bug report") {
		t.Error("Bug prompt should mention bug report")
	}
}
