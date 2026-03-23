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

// TestBuildTitleGenerationPrompt verifies the title generation prompt structure
func TestBuildTitleGenerationPrompt(t *testing.T) {
	technicalPlanning := "## Problem Statement\n\nAdd user authentication to the system.\n\n## Architecture Overview\n\nKey components involved: auth service, user database."

	prompt := BuildTitleGenerationPrompt(WizardTypeFeature, technicalPlanning, "en-US")

	// Verify prompt contains required elements
	if !strings.Contains(prompt, "Output ONLY the title text") {
		t.Error("Title prompt missing 'Output ONLY the title text' instruction")
	}
	if !strings.Contains(prompt, "maximum 80 characters") {
		t.Error("Title prompt missing max length constraint")
	}
	if !strings.Contains(prompt, "[Feature]") {
		t.Error("Title prompt missing [Feature] prefix requirement")
	}
	if !strings.Contains(prompt, technicalPlanning) {
		t.Error("Title prompt missing technical planning content")
	}
	if !strings.Contains(prompt, "Output MUST be in en-US") {
		t.Error("Title prompt missing language constraint")
	}
}

// TestBuildTitleGenerationPrompt_Bug verifies bug type title generation
func TestBuildTitleGenerationPrompt_Bug(t *testing.T) {
	technicalPlanning := "## Problem Statement\n\nFix login error when user enters wrong password."

	prompt := BuildTitleGenerationPrompt(WizardTypeBug, technicalPlanning, "en-US")

	if !strings.Contains(prompt, "[Bug]") {
		t.Error("Bug title prompt should require [Bug] prefix")
	}
	if !strings.Contains(prompt, "Issue Type: Bug") {
		t.Error("Bug title prompt should mention Bug type")
	}
}

// TestBuildTitleGenerationPrompt_NonEnglish verifies non-English language handling
func TestBuildTitleGenerationPrompt_NonEnglish(t *testing.T) {
	technicalPlanning := "## Problem Statement\n\nAdd user authentication."

	// Test with Polish
	prompt := BuildTitleGenerationPrompt(WizardTypeFeature, technicalPlanning, "pl-PL")
	if !strings.Contains(prompt, "Output MUST be in pl-PL") {
		t.Error("Title prompt should support Polish language")
	}

	// Test with German
	prompt = BuildTitleGenerationPrompt(WizardTypeFeature, technicalPlanning, "de-DE")
	if !strings.Contains(prompt, "Output MUST be in de-DE") {
		t.Error("Title prompt should support German language")
	}
}

// TestBuildTitleGenerationPrompt_DefaultLanguage verifies default language is English
func TestBuildTitleGenerationPrompt_DefaultLanguage(t *testing.T) {
	technicalPlanning := "## Problem Statement\n\nAdd user authentication."

	// Test with empty language - should default to en-US
	prompt := BuildTitleGenerationPrompt(WizardTypeFeature, technicalPlanning, "")
	if !strings.Contains(prompt, "Output MUST be in en-US") {
		t.Error("Title prompt should default to en-US when language is empty")
	}
}
