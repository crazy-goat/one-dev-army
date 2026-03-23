# Automatic Language Detection for Microphone Recordings - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use @superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement automatic browser language detection for speech recognition in the wizard, with manual override via dropdown and language-aware LLM prompts.

**Architecture:** Add `Language` field to `WizardSession`, detect browser language via `navigator.language`, persist to localStorage, pass language through form submission to backend, and modify LLM prompts to respect the selected language instead of hardcoding English.

**Tech Stack:** Go (backend), HTML/JavaScript (frontend), Web Speech API

---

## Overview

This plan implements GitHub issue #149: Automatic language detection for microphone recordings. Currently, speech recognition is hardcoded to `'en-US'` and LLM prompts force English output. This feature adds:

1. **Auto-detection:** Uses `navigator.language` to detect browser language
2. **Manual override:** Language selector dropdown in the UI
3. **Persistence:** localStorage for unauthenticated users
4. **Backend integration:** Language stored in session and passed to LLM
5. **Prompt engineering:** Remove hardcoded English requirement, make dynamic

**Supported Languages:** en-US, pl-PL, de-DE, es-ES, fr-FR, pt-PT, it-IT, nl-NL, ru-RU, zh-CN, ja-JP, ko-KR

---

## Task 1: Add Language Field to WizardSession

**Files:**
- Modify: `internal/dashboard/wizard.go:78-94`
- Test: `internal/dashboard/wizard_test.go` (add new test)

**Step 1: Write the failing test**

Add to `internal/dashboard/wizard_test.go`:

```go
func TestWizardSession_SetLanguage(t *testing.T) {
	session := &WizardSession{
		ID:   "test-session",
		Type: WizardTypeFeature,
	}
	
	session.SetLanguage("pl-PL")
	
	if session.Language != "pl-PL" {
		t.Errorf("Expected Language to be 'pl-PL', got %q", session.Language)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/dashboard -run TestWizardSession_SetLanguage -v
```

Expected: FAIL - `SetLanguage` method not defined

**Step 3: Add Language field and setter method**

Modify `internal/dashboard/wizard.go:78-94`:

```go
// WizardSession holds the state for a single wizard instance
type WizardSession struct {
	ID                 string     `json:"id"`
	Type               WizardType `json:"type"`
	CurrentStep        WizardStep `json:"current_step"`
	IdeaText           string     `json:"idea_text"`
	RefinedDescription string     `json:"refined_description"`
	TechnicalPlanning  string     `json:"technical_planning"`
	CreatedIssues     []CreatedIssue `json:"created_issues"`
	EpicNumber        int            `json:"epic_number"`
	AddToSprint       bool           `json:"add_to_sprint"`
	SkipBreakdown     bool           `json:"skip_breakdown"`
	Language          string         `json:"language"` // NEW FIELD
	LLMLogs           []LLMLogEntry  `json:"llm_logs"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	mu                sync.RWMutex   `json:"-"`
}
```

Add setter method after line 131 (after `SetTechnicalPlanning`):

```go
// SetLanguage updates the language preference (thread-safe)
func (s *WizardSession) SetLanguage(lang string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Language = lang
	s.UpdatedAt = time.Now()
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/dashboard -run TestWizardSession_SetLanguage -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/dashboard/wizard.go internal/dashboard/wizard_test.go
git commit -m "feat(wizard): add Language field to WizardSession with setter"
```

---

## Task 2: Update Prompts to Support Dynamic Language

**Files:**
- Modify: `internal/dashboard/prompts.go:20`
- Modify: `internal/dashboard/prompts.go:71-116`
- Modify: `internal/dashboard/prompts.go:170-189`
- Test: `internal/dashboard/prompts_test.go` (add new test)

**Step 1: Write the failing test**

Add to `internal/dashboard/prompts_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard -run TestBuildTechnicalPlanningPrompt_WithLanguage -v
```

Expected: FAIL - function signature doesn't accept language parameter

**Step 3: Update TechnicalPlanningPromptTemplate**

Modify `internal/dashboard/prompts.go:71-116` to add language instruction:

```go
// TechnicalPlanningPromptTemplate is the unified template for both refinement and technical analysis
// It outputs a structured technical planning document without implementation code
const TechnicalPlanningPromptTemplate = `You are a technical architect creating a GitHub issue with technical planning.

CRITICAL RULE: Output MUST be in %s regardless of input language.

Your output MUST be a markdown document with exactly these sections:

## Problem Statement / Feature Description
[Clear, professional description of what needs to be done]

## Architecture Overview
[High-level description of the system architecture needed]
- Key components involved
- Data flow overview
- Integration points

## Files Requiring Changes
[List specific file paths that will need modification]
- Path to each file with brief explanation of what changes are needed
- Include both existing files to modify and new files to create

## Component Dependencies
[Describe how components interact]
- Dependencies between modules
- External dependencies (libraries, APIs, services)
- Database schema changes if applicable

## Implementation Boundaries
[Clear boundaries of what to do and what NOT to do]
- What is in scope for this issue
- What is explicitly out of scope
- Constraints and limitations

## Acceptance Criteria
[2-4 specific, verifiable criteria for completion]

CRITICAL RULES:
- NO implementation code or algorithms
- NO specific technical solutions or design patterns
- NO "how to" instructions
- Focus on WHAT and WHERE, not HOW
- Be specific about file paths and component names
- Keep architecture description at a high level

Codebase context (for reference only):
%s

Original %s:
%s`
```

**Step 4: Update BuildTechnicalPlanningPrompt function signature**

Modify `internal/dashboard/prompts.go:170-189`:

```go
// BuildTechnicalPlanningPrompt creates the unified prompt for technical planning
// This combines refinement + technical analysis into a single LLM call
func BuildTechnicalPlanningPrompt(wizardType WizardType, idea string, codebaseContext string, language string) string {
	if codebaseContext == "" {
		codebaseContext = "No codebase context provided."
	}
	
	// Default to English if no language specified
	if language == "" {
		language = "en-US"
	}

	var typeLabel string
	if wizardType == WizardTypeBug {
		typeLabel = "bug report"
	} else {
		typeLabel = "feature request"
	}

	return fmt.Sprintf(TechnicalPlanningTemplate,
		language,          // %s - language requirement
		codebaseContext,
		typeLabel,
		idea,
	)
}
```

**Step 5: Update RefinementPromptTemplate to support language**

Modify `internal/dashboard/prompts.go:11-36`:

```go
// RefinementPromptTemplate is the base template for idea refinement
// It instructs the LLM to analyze the idea in the context of the existing codebase
const RefinementPromptTemplate = `You are a GitHub issue writer. Your ONLY output is a markdown issue body. You NEVER explain, narrate, or think out loud.

RULES:
- Output ONLY the issue body in markdown. Nothing else.
- Do NOT start with "Now I", "Let me", "Here's", "Based on", "I'll", "After analyzing", or ANY preamble.
- Do NOT include phrases like "comprehensive understanding", "I have analyzed", "Let me create".
- First character of your response MUST be "#" (a markdown heading) or "-" (a list item).
- Output MUST be in %s regardless of input language.

Codebase context (for your reference only, do NOT discuss it):
%s

Original %s:
%s

Write a professional GitHub issue body for this %s that covers:
1. %s
2. %s
3. %s
4. %s
5. %s
6. How it fits with existing codebase patterns

Format as a well-structured markdown %s suitable for a GitHub issue.`
```

**Step 6: Update BuildRefinementPrompt function**

Modify `internal/dashboard/prompts.go:118-154`:

```go
// BuildRefinementPrompt creates the prompt for idea refinement with codebase context
// wizardType: the type of wizard (feature or bug)
// idea: the original user idea
// codebaseContext: information about the existing codebase (file structure, key files, etc.)
// language: the output language (e.g., "en-US", "pl-PL")
func BuildRefinementPrompt(wizardType WizardType, idea string, codebaseContext string, language string) string {
	if codebaseContext == "" {
		codebaseContext = "No codebase context provided."
	}
	
	// Default to English if no language specified
	if language == "" {
		language = "en-US"
	}

	if wizardType == WizardTypeBug {
		return fmt.Sprintf(RefinementPromptTemplate,
			language,                      // %s - language requirement
			codebaseContext,               // %s - codebase context
			"bug description",             // %s - original type
			idea,                          // %s - original content
			"bug report",                  // %s - output type
			"description of the issue",    // %s - point 1
			"Steps to reproduce",          // %s - point 2
			"Expected vs actual behavior", // %s - point 3
			"Impact/severity assessment",  // %s - point 4
			"Any additional context that would help developers", // %s - point 5
			"bug report", // %s - final output type
		)
	}

	return fmt.Sprintf(RefinementPromptTemplate,
		language,                      // %s - language requirement
		codebaseContext,              // %s - codebase context
		"idea",                       // %s - original type
		idea,                         // %s - original content
		"feature description",        // %s - output type
		"problem statement",          // %s - point 1
		"Target users/personas",      // %s - point 2
		"Proposed solution overview", // %s - point 3
		"Key acceptance criteria",    // %s - point 4
		"Technical considerations or constraints", // %s - point 5
		"feature description",                     // %s - final output type
	)
}
```

**Step 7: Run tests to verify they pass**

```bash
go test ./internal/dashboard -run TestBuildTechnicalPlanningPrompt_WithLanguage -v
go test ./internal/dashboard -run TestBuildRefinementPrompt -v
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/dashboard/prompts.go internal/dashboard/prompts_test.go
git commit -m "feat(prompts): add language parameter to prompt builders"
```

---

## Task 3: Update Backend Handlers to Accept and Store Language

**Files:**
- Modify: `internal/dashboard/handlers.go:826-848` (handleWizardNew)
- Modify: `internal/dashboard/handlers.go:852-980` (handleWizardRefine)
- Test: `internal/dashboard/handlers_test.go` (add new tests)

**Step 1: Write the failing test**

Add to `internal/dashboard/handlers_test.go`:

```go
func TestHandleWizardRefine_AcceptsLanguageParameter(t *testing.T) {
	srv := NewTestServer(t)
	
	// First create a session
	req1 := httptest.NewRequest("GET", "/wizard/new?type=feature", nil)
	rec1 := httptest.NewRecorder()
	srv.handleWizardNew(rec1, req1)
	
	// Extract session ID from response
	body := rec1.Body.String()
	// Parse session_id from HTML form
	var sessionID string
	if strings.Contains(body, `name="session_id"`) {
		// Extract value from: value="..."
		start := strings.Index(body, `name="session_id" value="`)
		if start != -1 {
			start += len(`name="session_id" value="`)
			end := strings.Index(body[start:], `"`)
			if end != -1 {
				sessionID = body[start : start+end]
			}
		}
	}
	
	if sessionID == "" {
		t.Fatal("Could not extract session ID from response")
	}
	
	// Submit form with language parameter
	formData := url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Test feature idea")
	formData.Set("language", "pl-PL")
	
	req2 := httptest.NewRequest("POST", "/wizard/refine", strings.NewReader(formData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	
	srv.handleWizardRefine(rec2, req2)
	
	// Verify session has language stored
	session, ok := srv.wizardStore.Get(sessionID)
	if !ok {
		t.Fatal("Session not found")
	}
	
	if session.Language != "pl-PL" {
		t.Errorf("Expected Language to be 'pl-PL', got %q", session.Language)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard -run TestHandleWizardRefine_AcceptsLanguageParameter -v
```

Expected: FAIL - language parameter not being read or stored

**Step 3: Update handleWizardNew to pass language to template**

Modify `internal/dashboard/handlers.go:826-848`:

```go
data := struct {
	Type               string
	SessionID          string
	IsPage             bool
	CurrentStep        int
	ShowBreakdownStep  bool
	NeedsTypeSelection bool
	Language           string // NEW FIELD
}{
	Type:               wizardType,
	SessionID:          "",
	IsPage:             isPage,
	CurrentStep:        1,
	ShowBreakdownStep:  false,
	NeedsTypeSelection: needsTypeSelection,
	Language:           "", // Will be set from session if available
}

if session != nil {
	data.SessionID = session.ID
	data.Type = string(session.Type)
	data.ShowBreakdownStep = session.Type == WizardTypeFeature && !session.SkipBreakdown
	data.Language = session.Language // Pass stored language to template
}
```

**Step 4: Update handleWizardRefine to read and store language**

Modify `internal/dashboard/handlers.go:852-980` to add language handling after line 860:

```go
sessionID := r.FormValue("session_id")
idea := r.FormValue("idea")
currentDesc := r.FormValue("current_description")
language := r.FormValue("language") // NEW: Read language parameter

// ... existing validation code ...

// Store the idea using thread-safe setter (only if it's a new idea, not re-refinement)
if idea != "" {
	session.SetIdeaText(idea)
}

// Store language preference if provided
if language != "" {
	session.SetLanguage(language)
}
```

Then update the LLM call around line 956-957 to pass language:

```go
// Build unified technical planning prompt with codebase context
codebaseContext := GetCodebaseContext()
prompt := BuildTechnicalPlanningPrompt(session.Type, inputText, codebaseContext, session.Language)
session.AddLog("system", "Sending technical planning request to LLM (language: "+session.Language+")")
```

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/dashboard -run TestHandleWizardRefine_AcceptsLanguageParameter -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/dashboard/handlers.go internal/dashboard/handlers_test.go
git commit -m "feat(handlers): accept and store language parameter in wizard"
```

---

## Task 4: Add Frontend Language Detection and UI

**Files:**
- Modify: `internal/dashboard/templates/wizard_new.html:49-60` (add language selector)
- Modify: `internal/dashboard/templates/wizard_new.html:77-165` (update JavaScript)
- Test: Manual browser testing

**Step 1: Add language selector dropdown to form**

Modify `internal/dashboard/templates/wizard_new.html:49-60` to add language selector:

```html
<div class="form-group">
  <label for="idea">Describe your {{if eq .Type "bug"}}bug{{else}}feature idea{{end}}:</label>
  <div class="textarea-with-mic">
    <textarea id="idea" name="idea" rows="6" placeholder="{{if eq .Type "bug"}}Describe the bug, steps to reproduce, and expected behavior...{{else}}Describe the feature, who it's for, and what problem it solves...{{end}}" required></textarea>
    <button type="button" id="mic-btn" class="mic-btn" title="Voice input">
      <svg viewBox="0 0 24 24" width="20" height="20">
        <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z"/>
        <path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z"/>
      </svg>
    </button>
  </div>
</div>

<!-- Language Selector -->
<div class="form-group language-selector">
  <label for="language-select">Language for voice input:</label>
  <select id="language-select" name="language">
    <option value="en-US">🇺🇸 English</option>
    <option value="pl-PL">🇵🇱 Polski</option>
    <option value="de-DE">🇩🇪 Deutsch</option>
    <option value="es-ES">🇪🇸 Español</option>
    <option value="fr-FR">🇫🇷 Français</option>
    <option value="pt-PT">🇵🇹 Português</option>
    <option value="it-IT">🇮🇹 Italiano</option>
    <option value="nl-NL">🇳🇱 Nederlands</option>
    <option value="ru-RU">🇷🇺 Русский</option>
    <option value="zh-CN">🇨🇳 中文</option>
    <option value="ja-JP">🇯🇵 日本語</option>
    <option value="ko-KR">🇰🇷 한국어</option>
  </select>
  <input type="hidden" id="detected-language" value="{{.Language}}">
</div>
```

**Step 2: Add CSS for language selector**

Add to the `<style>` section at the end of `wizard_new.html`:

```css
/* Language selector styles */
.language-selector {
  margin-top: 0.5rem;
}

.language-selector label {
  font-size: 0.85rem;
  color: var(--muted);
}

.language-selector select {
  padding: 0.5rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface);
  color: var(--text);
  font-size: 0.9rem;
  cursor: pointer;
}

.language-selector select:focus {
  outline: none;
  border-color: var(--accent);
}
```

**Step 3: Update JavaScript for language detection and persistence**

Replace the entire `<script>` section in `internal/dashboard/templates/wizard_new.html:77-165`:

```javascript
<script>
(function() {
  const textarea = document.getElementById('idea');
  const micBtn = document.getElementById('mic-btn');
  const languageSelect = document.getElementById('language-select');
  const detectedLangInput = document.getElementById('detected-language');
  
  if (!textarea || !micBtn) return;
  
  const form = textarea.closest('form');
  
  // Supported languages mapping
  const supportedLanguages = {
    'en': 'en-US', 'en-US': 'en-US', 'en-GB': 'en-US',
    'pl': 'pl-PL', 'pl-PL': 'pl-PL',
    'de': 'de-DE', 'de-DE': 'de-DE',
    'es': 'es-ES', 'es-ES': 'es-ES', 'es-MX': 'es-ES',
    'fr': 'fr-FR', 'fr-FR': 'fr-FR',
    'pt': 'pt-PT', 'pt-PT': 'pt-PT', 'pt-BR': 'pt-PT',
    'it': 'it-IT', 'it-IT': 'it-IT',
    'nl': 'nl-NL', 'nl-NL': 'nl-NL',
    'ru': 'ru-RU', 'ru-RU': 'ru-RU',
    'zh': 'zh-CN', 'zh-CN': 'zh-CN', 'zh-TW': 'zh-CN',
    'ja': 'ja-JP', 'ja-JP': 'ja-JP',
    'ko': 'ko-KR', 'ko-KR': 'ko-KR'
  };
  
  // Detect browser language
  function detectBrowserLanguage() {
    // Try navigator.languages first (preferred order)
    if (navigator.languages && navigator.languages.length > 0) {
      for (const lang of navigator.languages) {
        const normalized = normalizeLanguage(lang);
        if (normalized) return normalized;
      }
    }
    
    // Fallback to navigator.language
    if (navigator.language) {
      const normalized = normalizeLanguage(navigator.language);
      if (normalized) return normalized;
    }
    
    // Default to English
    return 'en-US';
  }
  
  // Normalize language code to supported format
  function normalizeLanguage(lang) {
    if (!lang) return null;
    
    // Direct match
    if (supportedLanguages[lang]) {
      return supportedLanguages[lang];
    }
    
    // Try base language (e.g., "pl-PL" -> "pl")
    const baseLang = lang.split('-')[0];
    if (supportedLanguages[baseLang]) {
      return supportedLanguages[baseLang];
    }
    
    return null;
  }
  
  // Get stored language preference
  function getStoredLanguage() {
    try {
      return localStorage.getItem('wizard-language');
    } catch (e) {
      console.warn('localStorage not available:', e);
      return null;
    }
  }
  
  // Store language preference
  function storeLanguage(lang) {
    try {
      localStorage.setItem('wizard-language', lang);
    } catch (e) {
      console.warn('Could not store language preference:', e);
    }
  }
  
  // Initialize language selector
  function initializeLanguage() {
    // Priority: 1. Server-provided language, 2. localStorage, 3. Browser detection
    let selectedLang = detectedLangInput?.value;
    
    if (!selectedLang) {
      selectedLang = getStoredLanguage();
    }
    
    if (!selectedLang) {
      selectedLang = detectBrowserLanguage();
    }
    
    // Set the selector value
    if (languageSelect) {
      languageSelect.value = selectedLang;
      
      // Store for future visits
      storeLanguage(selectedLang);
      
      // Listen for changes
      languageSelect.addEventListener('change', function() {
        storeLanguage(this.value);
        
        // If recording, restart with new language
        if (isRecording && recognition) {
          stopRecording();
          recognition.lang = this.value;
          startRecording();
        }
      });
    }
    
    return selectedLang;
  }
  
  const currentLang = initializeLanguage();
  
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  
  if (!SpeechRecognition) {
    micBtn.style.display = 'none';
    if (languageSelect) {
      languageSelect.parentElement.style.display = 'none';
    }
    return;
  }
  
  let recognition = null;
  let isRecording = false;
  
  function initRecognition() {
    recognition = new SpeechRecognition();
    recognition.continuous = true;
    recognition.interimResults = true;
    recognition.lang = currentLang || 'en-US'; // Use detected/selected language
    
    recognition.onresult = (event) => {
      let finalTranscript = '';
      
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const transcript = event.results[i][0].transcript;
        if (event.results[i].isFinal) {
          finalTranscript += transcript;
        }
      }
      
      if (finalTranscript) {
        const start = textarea.selectionStart;
        const end = textarea.selectionEnd;
        const before = textarea.value.substring(0, start);
        const after = textarea.value.substring(end);
        
        textarea.value = before + finalTranscript + after;
        const newPos = start + finalTranscript.length;
        textarea.setSelectionRange(newPos, newPos);
      }
    };
    
    recognition.onerror = (event) => {
      console.error('Speech recognition error:', event.error);
      stopRecording();
    };
    
    recognition.onend = () => {
      if (isRecording) {
        recognition.start();
      }
    };
  }
  
  function startRecording() {
    if (!recognition) initRecognition();
    
    // Update language before starting (in case user changed it)
    if (languageSelect && recognition) {
      recognition.lang = languageSelect.value;
    }
    
    isRecording = true;
    micBtn.classList.add('recording');
    recognition.start();
  }
  
  function stopRecording() {
    isRecording = false;
    micBtn.classList.remove('recording');
    if (recognition) {
      recognition.stop();
    }
  }
  
  micBtn.addEventListener('click', () => {
    if (isRecording) {
      stopRecording();
    } else {
      startRecording();
    }
  });
  
  if (form) {
    form.addEventListener('submit', () => {
      if (isRecording) stopRecording();
    });
  }
})();
</script>
```

**Step 4: Test manually in browser**

```bash
# Start the server
cd /home/decodo/work/one-dev-army
go run ./cmd/dashboard

# Open browser and navigate to wizard
# Verify:
# 1. Language selector appears below textarea
# 2. Default language matches browser setting
# 3. Changing language updates localStorage
# 4. Voice recording uses selected language
```

**Step 5: Commit**

```bash
git add internal/dashboard/templates/wizard_new.html
git commit -m "feat(wizard): add language detection, selector, and persistence"
```

---

## Task 5: Update Handler Tests for Language Parameter

**Files:**
- Modify: `internal/dashboard/handlers_test.go` (update existing tests)

**Step 1: Update existing test to include language parameter**

Find and update `TestHandleWizardRefine_Success` in `internal/dashboard/handlers_test.go`:

```go
func TestHandleWizardRefine_Success(t *testing.T) {
	srv := NewTestServer(t)
	
	// Create a session first
	req1 := httptest.NewRequest("GET", "/wizard/new?type=feature", nil)
	rec1 := httptest.NewRecorder()
	srv.handleWizardNew(rec1, req1)
	
	// Extract session ID
	body := rec1.Body.String()
	var sessionID string
	if strings.Contains(body, `name="session_id"`) {
		start := strings.Index(body, `name="session_id" value="`)
		if start != -1 {
			start += len(`name="session_id" value="`)
			end := strings.Index(body[start:], `"`)
			if end != -1 {
				sessionID = body[start : start+end]
			}
		}
	}
	
	if sessionID == "" {
		t.Fatal("Could not extract session ID")
	}
	
	// Submit with language
	formData := url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Add user authentication feature")
	formData.Set("language", "en-US")
	
	req2 := httptest.NewRequest("POST", "/wizard/refine", strings.NewReader(formData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	
	srv.handleWizardRefine(rec2, req2)
	
	if rec2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec2.Code)
	}
	
	// Verify session stored language
	session, ok := srv.wizardStore.Get(sessionID)
	if !ok {
		t.Fatal("Session not found")
	}
	
	if session.Language != "en-US" {
		t.Errorf("Expected Language 'en-US', got %q", session.Language)
	}
}
```

**Step 2: Run all wizard-related tests**

```bash
go test ./internal/dashboard -run "Wizard" -v
```

Expected: All tests PASS

**Step 3: Commit**

```bash
git add internal/dashboard/handlers_test.go
git commit -m "test(handlers): update tests for language parameter"
```

---

## Task 6: Run Full Test Suite and Verify

**Step 1: Run all dashboard tests**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/dashboard/... -v 2>&1 | head -100
```

Expected: All tests PASS

**Step 2: Build and verify compilation**

```bash
go build ./cmd/dashboard
```

Expected: No compilation errors

**Step 3: Manual end-to-end testing checklist**

Start the server and verify:

- [ ] Language selector appears below textarea
- [ ] Default language matches browser setting (check `navigator.language` in console)
- [ ] Changing language persists after page refresh (check localStorage)
- [ ] Voice recording button works with different languages
- [ ] Form submission includes language parameter (check Network tab)
- [ ] LLM output respects selected language (test with non-English input)
- [ ] Fallback to English works for unsupported browser languages

**Step 4: Commit final changes**

```bash
git add .
git commit -m "feat(wizard): implement automatic language detection for microphone recordings

- Add Language field to WizardSession with thread-safe setter
- Update prompt builders to accept and use language parameter
- Modify handlers to accept, store, and pass language to LLM
- Add language selector UI with 12 supported languages
- Implement browser language detection via navigator.language
- Add localStorage persistence for language preference
- Update all tests to include language parameter

Closes #149"
```

---

## Summary

This implementation adds automatic language detection for microphone recordings in the wizard feature:

1. **Backend Changes:**
   - Added `Language` field to `WizardSession` struct
   - Updated `BuildTechnicalPlanningPrompt` and `BuildRefinementPrompt` to accept language
   - Modified `handleWizardRefine` to read and store language parameter
   - Updated all prompt templates to use dynamic language instead of hardcoded English

2. **Frontend Changes:**
   - Added language selector dropdown with 12 supported languages
   - Implemented browser language detection using `navigator.language`
   - Added localStorage persistence for language preference
   - Updated SpeechRecognition to use selected language

3. **Testing:**
   - Added unit tests for language field persistence
   - Added tests for prompt builders with language parameter
   - Updated handler tests to verify language parameter acceptance
   - Manual testing checklist for browser verification

**Breaking Changes:** None - all changes are backward compatible with default to English.

**Files Modified:**
- `internal/dashboard/wizard.go`
- `internal/dashboard/wizard_test.go`
- `internal/dashboard/prompts.go`
- `internal/dashboard/prompts_test.go`
- `internal/dashboard/handlers.go`
- `internal/dashboard/handlers_test.go`
- `internal/dashboard/templates/wizard_new.html`
