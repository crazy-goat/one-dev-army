package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func TestPlanSprint_ParseResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []int
		wantErr bool
	}{
		{
			name:    "clean json",
			content: `{"task_ids": [1, 5, 12]}`,
			want:    []int{1, 5, 12},
		},
		{
			name:    "json with surrounding text",
			content: "Here is the plan:\n{\"task_ids\": [3, 7]}\nDone.",
			want:    []int{3, 7},
		},
		{
			name:    "json in markdown code block",
			content: "```json\n{\"task_ids\": [42]}\n```",
			want:    []int{42},
		},
		{
			name:    "empty task list",
			content: `{"task_ids": []}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			content: "not json at all",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSprintPlan(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d task IDs, want %d", len(got), len(tt.want))
			}
			for i, id := range got {
				if id != tt.want[i] {
					t.Errorf("taskIDs[%d] = %d, want %d", i, id, tt.want[i])
				}
			}
		})
	}
}

func TestBuildSprintPrompt(t *testing.T) {
	type label = struct {
		Name string `json:"name"`
	}
	issues := []github.Issue{
		{
			Number: 1,
			Title:  "Add login page",
			Labels: []label{{Name: "size:M"}, {Name: "frontend"}},
		},
		{
			Number: 5,
			Title:  "Set up CI pipeline",
			Labels: []label{{Name: "size:S"}},
		},
		{
			Number: 10,
			Title:  "Database migration tool",
			Labels: []label{{Name: "backend"}},
		},
	}

	prompt := buildSprintPrompt(issues, 5)

	if !strings.Contains(prompt, "#1: Add login page [size:M]") {
		t.Error("prompt should contain issue #1 with size label")
	}
	if !strings.Contains(prompt, "#5: Set up CI pipeline [size:S]") {
		t.Error("prompt should contain issue #5 with size label")
	}
	if !strings.Contains(prompt, "#10: Database migration tool []") {
		t.Error("prompt should contain issue #10 with empty size")
	}
	if !strings.Contains(prompt, "up to 5 tasks") {
		t.Error("prompt should contain max tasks limit")
	}
	if !strings.Contains(prompt, "task_ids") {
		t.Error("prompt should instruct JSON format with task_ids")
	}
}

func TestBuildSprintPrompt_NoLimit(t *testing.T) {
	issues := []github.Issue{
		{Number: 1, Title: "Task 1"},
	}

	prompt := buildSprintPrompt(issues, 0)

	if strings.Contains(prompt, "up to") {
		t.Error("prompt should not contain limit when maxTasks is 0")
	}
}

func TestPlanSprint_Integration(t *testing.T) {
	planResponse := sprintPlanResponse{TaskIDs: []int{1, 5}}
	planJSON, _ := json.Marshal(planResponse)

	callCount := 0

	type planSSEClient struct {
		w       http.ResponseWriter
		flusher http.Flusher
	}
	var sseMu sync.Mutex
	var sseClients []*planSSEClient

	broadcast := func(data string) {
		sseMu.Lock()
		defer sseMu.Unlock()
		for _, c := range sseClients {
			fmt.Fprintf(c.w, "data: %s\n\n", data)
			c.flusher.Flush()
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/event" && r.Method == http.MethodGet {
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "data: %s\n\n", `{"type":"server.connected","properties":{}}`)
			flusher.Flush()

			c := &planSSEClient{w: w, flusher: flusher}
			sseMu.Lock()
			sseClients = append(sseClients, c)
			sseMu.Unlock()

			<-r.Context().Done()
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/session" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(opencode.Session{ID: "sess-plan", Title: "sprint-planning"})

		case strings.HasSuffix(r.URL.Path, "/prompt_async") && r.Method == http.MethodPost:
			callCount++
			escapedJSON := strings.ReplaceAll(string(planJSON), `"`, `\"`)
			w.WriteHeader(http.StatusNoContent)
			go func() {
				broadcast(`{"type":"message.updated","properties":{"info":{"id":"msg-1","sessionID":"sess-plan","role":"assistant"}}}`)
				broadcast(fmt.Sprintf(`{"type":"message.part.delta","properties":{"sessionID":"sess-plan","messageID":"msg-1","partID":"prt-1","field":"text","delta":"%s"}}`, escapedJSON))
				broadcast(`{"type":"session.status","properties":{"sessionID":"sess-plan","status":{"type":"idle"}}}`)
			}()

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.Config{
		Planning: config.Planning{LLM: "test-planner"},
		Sprint:   config.Sprint{TasksPerSprint: 5},
	}
	oc := opencode.NewClient(srv.URL)

	planner := NewPlanner(cfg, oc, nil, nil, nil)

	session, err := planner.oc.CreateSession("sprint-planning")
	if err != nil {
		t.Fatalf("creating session: %v", err)
	}

	type label = struct {
		Name string `json:"name"`
	}
	prompt := buildSprintPrompt([]github.Issue{
		{Number: 1, Title: "Task A", Labels: []label{{Name: "size:S"}}},
		{Number: 5, Title: "Task B", Labels: []label{{Name: "size:M"}}},
	}, cfg.Sprint.TasksPerSprint)

	msg, err := planner.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(cfg.Planning.LLM), nil)
	if err != nil {
		t.Fatalf("sending message: %v", err)
	}

	content := extractTextContent(msg)
	taskIDs, err := parseSprintPlan(content)
	if err != nil {
		t.Fatalf("parsing plan: %v", err)
	}

	if len(taskIDs) != 2 {
		t.Fatalf("got %d task IDs, want 2", len(taskIDs))
	}
	if taskIDs[0] != 1 || taskIDs[1] != 5 {
		t.Errorf("taskIDs = %v, want [1, 5]", taskIDs)
	}
}

func TestBuildInsightPrompt(t *testing.T) {
	insights := []string{
		"Issue #1 (Login): insight: should add rate limiting",
		"Issue #5 (CI): observation: builds are slow",
	}

	prompt := buildInsightPrompt(insights)

	if !strings.Contains(prompt, "rate limiting") {
		t.Error("prompt should contain first insight")
	}
	if !strings.Contains(prompt, "builds are slow") {
		t.Error("prompt should contain second insight")
	}
	if !strings.Contains(prompt, "concrete_ideas") {
		t.Error("prompt should instruct JSON format with concrete_ideas")
	}
	if !strings.Contains(prompt, "observations") {
		t.Error("prompt should instruct JSON format with observations")
	}
}

func TestParseInsightAnalysis(t *testing.T) {
	content := `{
		"concrete_ideas": [
			{"title": "Add rate limiting", "description": "Implement rate limiting on auth endpoints"}
		],
		"observations": [
			"Build times have increased 30% over the last sprint",
			"Test coverage is declining"
		]
	}`

	analysis, err := parseInsightAnalysis(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(analysis.ConcreteIdeas) != 1 {
		t.Fatalf("got %d ideas, want 1", len(analysis.ConcreteIdeas))
	}
	if analysis.ConcreteIdeas[0].Title != "Add rate limiting" {
		t.Errorf("idea title = %q, want %q", analysis.ConcreteIdeas[0].Title, "Add rate limiting")
	}
	if analysis.ConcreteIdeas[0].Description != "Implement rate limiting on auth endpoints" {
		t.Errorf("idea description = %q", analysis.ConcreteIdeas[0].Description)
	}

	if len(analysis.Observations) != 2 {
		t.Fatalf("got %d observations, want 2", len(analysis.Observations))
	}
	if analysis.Observations[0] != "Build times have increased 30% over the last sprint" {
		t.Errorf("observation[0] = %q", analysis.Observations[0])
	}
}

func TestParseInsightAnalysis_WithMarkdown(t *testing.T) {
	content := "```json\n{\"concrete_ideas\": [], \"observations\": [\"note 1\"]}\n```"

	analysis, err := parseInsightAnalysis(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(analysis.ConcreteIdeas) != 0 {
		t.Errorf("got %d ideas, want 0", len(analysis.ConcreteIdeas))
	}
	if len(analysis.Observations) != 1 {
		t.Fatalf("got %d observations, want 1", len(analysis.Observations))
	}
}

func TestParseInsightAnalysis_Invalid(t *testing.T) {
	_, err := parseInsightAnalysis("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
