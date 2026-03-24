package scheduler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func TestEpicAnalyzer_ParseResponse(t *testing.T) {
	tasks := []TaskSpec{
		{
			Title:              "Set up database schema",
			TechnicalDesc:      "Create PostgreSQL tables for users and sessions",
			AcceptanceCriteria: []string{"Migration runs without errors", "Tables have correct indexes"},
			Size:               "M",
			Dependencies:       nil,
			Labels:             []string{"backend"},
		},
		{
			Title:              "Implement auth endpoints",
			TechnicalDesc:      "Create login and register REST endpoints",
			AcceptanceCriteria: []string{"POST /login returns JWT", "POST /register creates user"},
			Size:               "L",
			Dependencies:       []int{1},
			Labels:             []string{"backend", "auth"},
		},
	}

	responseJSON, err := json.Marshal(tasks)
	if err != nil {
		t.Fatalf("marshaling test data: %v", err)
	}

	var callCount atomic.Int32

	type sseClient struct {
		w       http.ResponseWriter
		flusher http.Flusher
		done    chan struct{} // signals when SSE connection should close
	}
	var sseMu sync.Mutex
	var sseClients []*sseClient
	var broadcastWg sync.WaitGroup

	broadcast := func(data string) {
		sseMu.Lock()
		defer sseMu.Unlock()
		for _, c := range sseClients {
			_, _ = fmt.Fprintf(c.w, "data: %s\n\n", data)
			c.flusher.Flush()
		}
	}

	// Collect errors from HTTP handler goroutine to check in the test goroutine
	var handlerErrors []string
	var handlerErrMu sync.Mutex

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
			_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"type":"server.connected","properties":{}}`)
			flusher.Flush()

			done := make(chan struct{})
			c := &sseClient{w: w, flusher: flusher, done: done}
			sseMu.Lock()
			sseClients = append(sseClients, c)
			sseMu.Unlock()

			// Wait for either context cancellation or explicit done signal.
			// We must ensure all broadcast goroutines finish writing to this
			// ResponseWriter before the handler returns.
			select {
			case <-r.Context().Done():
			case <-done:
			}
			// Wait for any in-flight broadcast goroutines to complete
			broadcastWg.Wait()
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/session" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(opencode.Session{ID: "sess-epic", Title: "epic-analysis"})

		case strings.HasSuffix(r.URL.Path, "/prompt_async") && r.Method == http.MethodPost:
			callCount.Add(1)
			body, _ := io.ReadAll(r.Body)
			var req opencode.SendMessageRequest
			if err := json.Unmarshal(body, &req); err != nil {
				handlerErrMu.Lock()
				handlerErrors = append(handlerErrors, fmt.Sprintf("unmarshaling request: %v", err))
				handlerErrMu.Unlock()
				return
			}
			if req.Model == nil || req.Model.ModelID != "test-model" {
				handlerErrMu.Lock()
				handlerErrors = append(handlerErrors, fmt.Sprintf("model = %+v, want modelID=test-model", req.Model))
				handlerErrMu.Unlock()
			}
			if !strings.Contains(req.Parts[0].Text, "Build a user management system") {
				handlerErrMu.Lock()
				handlerErrors = append(handlerErrors, "prompt should contain epic description")
				handlerErrMu.Unlock()
			}

			escapedJSON := strings.ReplaceAll(string(responseJSON), `"`, `\"`)

			// Broadcast SSE events in a goroutine, but track it so the SSE
			// handler can wait for completion before returning.
			broadcastWg.Go(func() {
				broadcast(`{"type":"message.updated","properties":{"info":{"id":"msg-1","sessionID":"sess-epic","role":"assistant"}}}`)
				broadcast(fmt.Sprintf(`{"type":"message.part.delta","properties":{"sessionID":"sess-epic","messageID":"msg-1","partID":"prt-1","field":"text","delta":"%s"}}`, escapedJSON))
				broadcast(`{"type":"session.status","properties":{"sessionID":"sess-epic","status":{"type":"idle"}}}`)
			})

			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.Config{
		EpicAnalysis: config.EpicAnalysis{LLM: "test-model"},
	}
	oc := opencode.NewClient(srv.URL)
	router := llm.NewRouter(&cfg.LLM)
	ea := NewEpicAnalyzer(cfg, oc, nil, router)

	result, err := ea.Analyze("Build a user management system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d tasks, want 2", len(result))
	}

	if result[0].Title != "Set up database schema" {
		t.Errorf("task[0].Title = %q, want %q", result[0].Title, "Set up database schema")
	}
	if result[0].Size != "M" {
		t.Errorf("task[0].Size = %q, want %q", result[0].Size, "M")
	}
	if len(result[0].AcceptanceCriteria) != 2 {
		t.Errorf("task[0].AcceptanceCriteria length = %d, want 2", len(result[0].AcceptanceCriteria))
	}

	if result[1].Title != "Implement auth endpoints" {
		t.Errorf("task[1].Title = %q, want %q", result[1].Title, "Implement auth endpoints")
	}
	if len(result[1].Dependencies) != 1 || result[1].Dependencies[0] != 1 {
		t.Errorf("task[1].Dependencies = %v, want [1]", result[1].Dependencies)
	}
	if len(result[1].Labels) != 2 {
		t.Errorf("task[1].Labels = %v, want [backend auth]", result[1].Labels)
	}

	if got := callCount.Load(); got != 1 {
		t.Errorf("expected 1 message call, got %d", got)
	}

	// Check for errors collected from HTTP handler goroutine
	handlerErrMu.Lock()
	errs := handlerErrors
	handlerErrMu.Unlock()
	for _, e := range errs {
		t.Errorf("handler error: %s", e)
	}
}

func TestEpicAnalyzer_ParseResponseWithMarkdownWrapper(t *testing.T) {
	rawJSON := `[{"title":"Task A","technical_description":"Do A","acceptance_criteria":["AC1"],"size":"S","dependencies":[],"labels":[]}]`
	wrapped := "```json\n" + rawJSON + "\n```"

	tasks, err := parseTaskSpecs(wrapped)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].Title != "Task A" {
		t.Errorf("title = %q, want %q", tasks[0].Title, "Task A")
	}
}

func TestTaskSpecToIssueBody(t *testing.T) {
	task := TaskSpec{
		Title:              "Implement caching layer",
		TechnicalDesc:      "Add Redis-based caching for API responses",
		AcceptanceCriteria: []string{"Cache hit returns in <10ms", "Cache invalidation works on write"},
		Size:               "L",
		Dependencies:       []int{1, 3},
		Labels:             []string{"backend", "performance"},
	}

	body := formatTaskBody(task)

	if !strings.Contains(body, "## Technical Description") {
		t.Error("body should contain Technical Description header")
	}
	if !strings.Contains(body, "Add Redis-based caching for API responses") {
		t.Error("body should contain technical description text")
	}
	if !strings.Contains(body, "## Acceptance Criteria") {
		t.Error("body should contain Acceptance Criteria header")
	}
	if !strings.Contains(body, "- [ ] Cache hit returns in <10ms") {
		t.Error("body should contain first acceptance criterion as checkbox")
	}
	if !strings.Contains(body, "- [ ] Cache invalidation works on write") {
		t.Error("body should contain second acceptance criterion as checkbox")
	}
	if !strings.Contains(body, "## Dependencies") {
		t.Error("body should contain Dependencies header")
	}
	if !strings.Contains(body, "- Task 1") {
		t.Error("body should contain dependency on task 1")
	}
	if !strings.Contains(body, "- Task 3") {
		t.Error("body should contain dependency on task 3")
	}
}

func TestTaskSpecToIssueBody_NoDependencies(t *testing.T) {
	task := TaskSpec{
		Title:              "Simple task",
		TechnicalDesc:      "Do something simple",
		AcceptanceCriteria: []string{"It works"},
		Size:               "S",
	}

	body := formatTaskBody(task)

	if strings.Contains(body, "## Dependencies") {
		t.Error("body should not contain Dependencies header when there are none")
	}
}

func TestBuildTaskLabels(t *testing.T) {
	task := TaskSpec{
		Size:   "M",
		Labels: []string{"backend", "auth"},
	}

	labels := buildTaskLabels(task)

	if len(labels) != 3 {
		t.Fatalf("got %d labels, want 3", len(labels))
	}
	if labels[0] != "size:M" {
		t.Errorf("labels[0] = %q, want %q", labels[0], "size:M")
	}
	if labels[1] != "backend" {
		t.Errorf("labels[1] = %q, want %q", labels[1], "backend")
	}
	if labels[2] != "auth" {
		t.Errorf("labels[2] = %q, want %q", labels[2], "auth")
	}
}

func TestBuildTaskLabels_NoSize(t *testing.T) {
	task := TaskSpec{
		Labels: []string{"frontend"},
	}

	labels := buildTaskLabels(task)

	if len(labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(labels))
	}
	if labels[0] != "frontend" {
		t.Errorf("labels[0] = %q, want %q", labels[0], "frontend")
	}
}

func TestExtractJSON_Array(t *testing.T) {
	input := `Here is the result:\n[{"title":"test"}]\nDone.`
	result := extractJSON(input)
	if result != `[{"title":"test"}]` {
		t.Errorf("got %q, want %q", result, `[{"title":"test"}]`)
	}
}

func TestExtractJSON_Object(t *testing.T) {
	input := `Result: {"task_ids": [1, 2]}`
	result := extractJSON(input)
	if result != `{"task_ids": [1, 2]}` {
		t.Errorf("got %q, want %q", result, `{"task_ids": [1, 2]}`)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	input := "no json here"
	result := extractJSON(input)
	if result != input {
		t.Errorf("got %q, want %q", result, input)
	}
}

func TestExtractTextContent(t *testing.T) {
	msg := &opencode.Message{
		Parts: []opencode.Part{
			{Type: "tool_call", Text: ""},
			{Type: "text", Text: "hello world"},
		},
	}
	result := extractTextContent(msg)
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}

func TestExtractTextContent_Empty(t *testing.T) {
	msg := &opencode.Message{
		Parts: []opencode.Part{
			{Type: "tool_call", Text: "something"},
		},
	}
	result := extractTextContent(msg)
	if result != "" {
		t.Errorf("got %q, want empty", result)
	}
}
