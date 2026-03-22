package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

type CRResult struct {
	Approved    bool     `json:"approved"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
	Verdict     string   `json:"verdict"`
}

type jsonEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionID"`
	Part      struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"part"`
}

func main() {
	prompt := buildCRPrompt()
	fmt.Printf("--- PROMPT (%d chars) ---\n%s\n---\n\n", len(prompt), prompt)

	cmd := exec.Command("opencode", "run",
		"--attach", "http://localhost:4096",
		"--format", "json",
		prompt,
	)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("start: %v", err)
	}

	var textParts []string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		var evt jsonEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		switch evt.Type {
		case "text":
			textParts = append(textParts, evt.Part.Text)
			fmt.Print(evt.Part.Text)
		case "tool_call":
			fmt.Printf("\n[tool_call]\n")
		case "tool_result":
			fmt.Printf("[tool_result]\n")
		case "step_start":
			fmt.Printf("[step_start]\n")
		case "step_finish":
			fmt.Printf("\n[step_finish]\n")
		case "error":
			fmt.Printf("\n[ERROR] %s\n", line)
		}
	}

	cmd.Wait()

	fullText := strings.Join(textParts, "")
	fmt.Printf("\n\n--- FULL TEXT (%d chars) ---\n%s\n", len(fullText), fullText)

	fmt.Println("\n--- JSON EXTRACTION ---")
	result, err := extractJSON(fullText)
	if err != nil {
		fmt.Printf("✗ %v\n", err)
		os.Exit(1)
	}
	pretty, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("✓ approved=%v\n%s\n", result.Approved, string(pretty))
}

func buildCRPrompt() string {
	return `Review this simple Go function:

` + "```go" + `
func Add(a, b int) int {
    return a + b
}

func TestAdd(t *testing.T) {
    if Add(2, 3) != 5 {
        t.Error("expected 5")
    }
}
` + "```" + `

Check for: correctness, code quality, error handling, tests, security, performance.

Your ENTIRE response MUST be ONLY a single JSON object with NO other text before or after it.
Do NOT include any explanation, reasoning, or markdown formatting.
Do NOT wrap the JSON in code blocks.
Respond with ONLY this JSON structure:

{"approved": true_or_false, "issues": ["list of issues"], "suggestions": ["list of suggestions"], "verdict": "one sentence summary"}
`
}

func extractJSON(text string) (*CRResult, error) {
	text = strings.TrimSpace(text)

	if strings.HasPrefix(text, "{") {
		var result CRResult
		if err := json.Unmarshal([]byte(text), &result); err == nil {
			return &result, nil
		}
	}

	if idx := strings.LastIndex(text, "```json"); idx >= 0 {
		start := idx + 7
		end := strings.Index(text[start:], "```")
		if end >= 0 {
			jsonStr := strings.TrimSpace(text[start : start+end])
			var result CRResult
			if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
				return &result, nil
			}
		}
	}

	if idx := strings.LastIndex(text, "{"); idx >= 0 {
		for end := len(text); end > idx; end-- {
			if text[end-1] == '}' {
				var result CRResult
				if err := json.Unmarshal([]byte(text[idx:end]), &result); err == nil {
					return &result, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no valid JSON found in response")
}
