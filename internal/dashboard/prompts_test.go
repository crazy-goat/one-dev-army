package dashboard

import (
	"strings"
	"testing"
)

// TestRefinementPrompt_EnglishOnlyConstraint verifies prompt requires English output by default
func TestRefinementPrompt_EnglishOnlyConstraint(t *testing.T) {
	if !strings.Contains(RefinementPromptTemplate, "Output MUST be in %s regardless of input language") {
		t.Error("RefinementPromptTemplate must contain dynamic language constraint")
	}
}

func TestBuildTechnicalPlanningPrompt(t *testing.T) {
	prompt := BuildTechnicalPlanningPrompt(WizardTypeFeature, "Add user authentication", "Go web service", "en-US")

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
	prompt := BuildTechnicalPlanningPrompt(WizardTypeBug, "Fix login error", "Go web service", "en-US")

	if !strings.Contains(prompt, "bug report") {
		t.Error("Bug prompt should mention bug report")
	}
}

func TestBuildTechnicalPlanningPrompt_WithLanguage(t *testing.T) {
	idea := "Add user authentication"
	codebaseContext := "Go backend service"

	// Test with Polish language
	prompt := BuildTechnicalPlanningPrompt(WizardTypeFeature, idea, codebaseContext, "pl-PL")

	if !strings.Contains(prompt, "Output MUST be in pl-PL") {
		t.Errorf("Expected prompt to contain language instruction for pl-PL")
	}

	// Test with English language
	prompt = BuildTechnicalPlanningPrompt(WizardTypeFeature, idea, codebaseContext, "en-US")

	if !strings.Contains(prompt, "Output MUST be in en-US") {
		t.Errorf("Expected prompt to contain language instruction for en-US")
	}
}
