package dashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/worker"
	"github.com/google/uuid"
)

type taskCard struct {
	ID       int
	Title    string
	Status   string
	Worker   string
	Assignee string
	Labels   []string
	PRURL    string
}

type boardData struct {
	Active       string
	SprintName   string
	Paused       bool
	Processing   bool
	CurrentIssue string
	Backlog      []taskCard
	Progress     []taskCard
	AIReview     []taskCard
	Approve      []taskCard
	Done         []taskCard
	Blocked      []taskCard
	Failed       []taskCard
}

type backlogData struct {
	Active string
	Items  []taskCard
}

type costsData struct {
	Active string
}

type workerCard struct {
	ID      string
	Status  string
	TaskID  int
	Task    string
	Stage   string
	Elapsed string
}

func placeholderBoard() boardData {
	return boardData{
		Active: "board",
		Backlog: []taskCard{
			{ID: 1, Title: "Set up CI pipeline", Status: "backlog"},
			{ID: 2, Title: "Add logging middleware", Status: "backlog"},
		},
		Progress: []taskCard{
			{ID: 3, Title: "Implement auth service", Status: "in_progress", Worker: "worker-1"},
		},
		AIReview: []taskCard{
			{ID: 4, Title: "Database migrations", Status: "ai_review"},
		},
		Approve: []taskCard{},
		Done: []taskCard{
			{ID: 5, Title: "Project skeleton", Status: "done"},
		},
		Blocked: []taskCard{
			{ID: 6, Title: "Deploy to staging", Status: "blocked"},
		},
	}
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	data := s.buildBoardData()
	s.render(w, "board.html", data)
}

func (s *Server) handleBoardData(w http.ResponseWriter, r *http.Request) {
	data := s.buildBoardData()
	s.renderFragment(w, "board.html", data)
}

func (s *Server) buildBoardData() boardData {
	data := boardData{
		Active: "board",
		Paused: true,
	}

	if s.orchestrator != nil {
		data.Paused = s.orchestrator.IsPaused()
		data.Processing = s.orchestrator.IsProcessing()
		if task := s.orchestrator.CurrentTask(); task != nil {
			data.CurrentIssue = fmt.Sprintf("#%d: %s (%s)", task.Issue.Number, task.Issue.Title, task.Status)
		}
	}

	// Get active milestone name
	if s.gh != nil && s.gh.GetActiveMilestone() != nil {
		data.SprintName = s.gh.GetActiveMilestone().Title
		log.Printf("[Dashboard] Active milestone: %s", data.SprintName)
	} else {
		log.Printf("[Dashboard] No active milestone set (gh=%v)", s.gh != nil)
	}

	// If no GitHub client or no active milestone, return empty board
	if s.gh == nil || s.gh.GetActiveMilestone() == nil {
		return data
	}

	milestone := s.gh.GetActiveMilestone().Title

	// Fetch issues from the active milestone
	issues, err := s.gh.ListIssuesForMilestone(milestone)
	if err != nil {
		log.Printf("[Dashboard] Error fetching issues for milestone %s: %v", milestone, err)
		return data
	}
	log.Printf("[Dashboard] Found %d issues in milestone %s", len(issues), milestone)

	// Fetch project items with their status
	itemsByStatus, err := s.gh.GetProjectItemsByStatus(s.projectNumber)
	if err != nil {
		log.Printf("[Dashboard] Error fetching project items for project %d: %v", s.projectNumber, err)
		itemsByStatus = make(map[string][]github.ProjectItem)
		for _, col := range github.ProjectColumns {
			itemsByStatus[col] = []github.ProjectItem{}
		}
	} else {
		totalItems := 0
		for _, items := range itemsByStatus {
			totalItems += len(items)
		}
		log.Printf("[Dashboard] Found %d items in project %d", totalItems, s.projectNumber)

		// If project is empty, we'll map issues to columns based on their state/labels
		if totalItems == 0 {
			itemsByStatus = nil // Signal that we need to infer status from issues
		}
	}

	// Create a map of issue number to issue for quick lookup
	issueMap := make(map[int]github.Issue)
	for _, issue := range issues {
		issueMap[issue.Number] = issue
	}

	// Build task cards for each column
	if itemsByStatus != nil {
		seen := make(map[int]bool)
		for _, col := range github.ProjectColumns {
			items := itemsByStatus[col]
			for _, item := range items {
				if item.Number == 0 {
					continue
				}
				seen[item.Number] = true
				issue, exists := issueMap[item.Number]
				if !exists {
					issue = github.Issue{
						Number: item.Number,
						Title:  item.Title,
					}
				}
				s.addCardToColumn(&data, col, issue)
			}
		}
		for _, issue := range issues {
			if seen[issue.Number] {
				continue
			}
			col := inferColumnFromIssue(issue)
			s.addCardToColumn(&data, col, issue)
		}
	} else {
		for _, issue := range issues {
			col := inferColumnFromIssue(issue)
			s.addCardToColumn(&data, col, issue)
		}
	}

	return data
}

// activeSprintName returns the title of the active sprint milestone, or empty string if none.
func (s *Server) activeSprintName() string {
	if s.gh != nil && s.gh.GetActiveMilestone() != nil {
		return s.gh.GetActiveMilestone().Title
	}
	return ""
}

func inferColumnFromIssue(issue github.Issue) string {
	// Check labels first
	labels := issue.GetLabelNames()
	labelSet := make(map[string]bool)
	for _, l := range labels {
		labelSet[strings.ToLower(l)] = true
	}

	// Map labels to columns
	if labelSet["failed"] {
		return "Failed"
	}
	if labelSet["blocked"] || labelSet["blocker"] {
		return "Blocked"
	}
	if labelSet["in-progress"] || labelSet["in progress"] || labelSet["wip"] || labelSet["working"] {
		return "In Progress"
	}
	if labelSet["review"] || labelSet["in-review"] || labelSet["pr-ready"] {
		return "AI Review"
	}
	if labelSet["awaiting-approval"] || labelSet["approve"] || labelSet["merge-ready"] {
		return "Approve"
	}
	if labelSet["done"] || labelSet["completed"] || labelSet["finished"] {
		return "Done"
	}

	if strings.EqualFold(issue.State, "CLOSED") {
		return "Done"
	}

	// Default to Backlog
	return "Backlog"
}

func (s *Server) addCardToColumn(data *boardData, col string, issue github.Issue) {
	card := taskCard{
		ID:       issue.Number,
		Title:    issue.Title,
		Status:   col,
		Assignee: issue.GetAssignee(),
		Labels:   issue.GetLabelNames(),
	}

	switch col {
	case "Backlog":
		data.Backlog = append(data.Backlog, card)
	case "In Progress":
		data.Progress = append(data.Progress, card)
	case "AI Review":
		data.AIReview = append(data.AIReview, card)
	case "Approve":
		if s.store != nil {
			if prURL, err := s.store.GetStepResponse(issue.Number, "create-pr"); err == nil && prURL != "" {
				card.PRURL = prURL
			}
		}
		data.Approve = append(data.Approve, card)
	case "Done":
		data.Done = append(data.Done, card)
	case "Blocked":
		data.Blocked = append(data.Blocked, card)
	case "Failed":
		data.Failed = append(data.Failed, card)
	}
}

func (s *Server) handleBacklog(w http.ResponseWriter, r *http.Request) {
	data := backlogData{
		Active: "backlog",
		Items: []taskCard{
			{ID: 10, Title: "Add rate limiting", Status: "backlog"},
			{ID: 11, Title: "Write API docs", Status: "backlog"},
			{ID: 12, Title: "Set up monitoring", Status: "backlog"},
		},
	}
	s.render(w, "backlog.html", data)
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	data := costsData{Active: "costs"}
	s.render(w, "costs.html", data)
}

func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	var cards []workerCard
	if s.pool != nil {
		for _, info := range s.pool() {
			cards = append(cards, toWorkerCard(info))
		}
	}
	if len(cards) == 0 {
		cards = placeholderWorkers()
	}
	s.render(w, "workers.html", cards)
}

func (s *Server) handleAddEpic(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/backlog", http.StatusSeeOther)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if s.gh != nil {
		m, err := s.gh.GetOldestOpenMilestone()
		if err != nil {
			log.Printf("[Dashboard] Sync error: %v", err)
		} else {
			s.gh.SetActiveMilestone(m)
			if m != nil {
				log.Printf("[Dashboard] Synced active milestone: %s", m.Title)
			} else {
				log.Printf("[Dashboard] Synced: no active milestone")
			}
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handlePlanSprint(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := s.gh.RemoveLabel(issueNum, "failed"); err != nil {
		log.Printf("[Dashboard] Error removing failed label from #%d: %v", issueNum, err)
	}

	log.Printf("[Dashboard] Retry #%d — will resume from last step", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleRetryFresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Close any open PR and delete branch
	if branch, err := s.gh.FindPRBranch(issueNum); err == nil {
		log.Printf("[Dashboard] Closing PR for #%d (branch: %s)", issueNum, branch)
		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[Dashboard] Error closing PR for #%d: %v", issueNum, closeErr)
		}
	}

	// Remove all workflow labels
	for _, label := range []string{"failed", "in-progress", "awaiting-approval"} {
		if err := s.gh.RemoveLabel(issueNum, label); err != nil {
			log.Printf("[Dashboard] Error removing %s label from #%d: %v", label, issueNum, err)
		}
	}

	// Clear DB steps
	if s.store != nil {
		if err := s.store.DeleteSteps(issueNum); err != nil {
			log.Printf("[Dashboard] Error deleting steps for #%d: %v", issueNum, err)
		}
	}

	log.Printf("[Dashboard] Retry fresh #%d — PR closed, steps cleared, starting from scratch", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) recordStep(issueNum int, stepName, response string) {
	if s.store == nil {
		return
	}
	id, err := s.store.InsertStep(issueNum, stepName, stepName, "")
	if err != nil {
		log.Printf("[Dashboard] failed to insert %s step for #%d: %v", stepName, issueNum, err)
		return
	}
	_ = s.store.FinishStep(id, response)
}

func (s *Server) handleDecline(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	reason := r.FormValue("reason")

	s.recordStep(issueNum, "declined", reason)

	if err := s.gh.RemoveLabel(issueNum, "awaiting-approval"); err != nil {
		log.Printf("[Dashboard] Error removing awaiting-approval label from #%d: %v", issueNum, err)
	}

	if reason != "" {
		comment := fmt.Sprintf("**Declined** — sent back for fixes.\n\n%s", reason)
		if err := s.gh.AddComment(issueNum, comment); err != nil {
			log.Printf("[Dashboard] Error adding decline comment to #%d: %v", issueNum, err)
		}
	}

	if s.store != nil {
		if err := s.store.DeleteSteps(issueNum); err != nil {
			log.Printf("[Dashboard] Error deleting steps for #%d: %v", issueNum, err)
		}
	}

	log.Printf("[Dashboard] Declined #%d — reason: %s", issueNum, reason)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleApproveMerge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	branch, err := s.gh.FindPRBranch(issueNum)
	if err != nil {
		log.Printf("[Dashboard] Error finding PR for #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.recordStep(issueNum, "approved", "Manual approval granted")

	log.Printf("[Dashboard] Approving & merging PR for #%d (branch: %s)", issueNum, branch)
	if err := s.gh.MergePR(branch); err != nil {
		log.Printf("[Dashboard] ✗ Merge failed for #%d (likely conflict): %v", issueNum, err)
		s.recordStep(issueNum, "merge-failed", err.Error())

		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[Dashboard] Error closing PR for #%d: %v", issueNum, closeErr)
		}

		if rmErr := s.gh.RemoveLabel(issueNum, "awaiting-approval"); rmErr != nil {
			log.Printf("[Dashboard] Error removing awaiting-approval label from #%d: %v", issueNum, rmErr)
		}
		if s.store != nil {
			if delErr := s.store.DeleteSteps(issueNum); delErr != nil {
				log.Printf("[Dashboard] Error deleting steps for #%d: %v", issueNum, delErr)
			}
		}

		comment := fmt.Sprintf("Merge failed (likely conflict). PR closed, task reset for fresh start.\n\nError: %s", err.Error())
		if cmtErr := s.gh.AddComment(issueNum, comment); cmtErr != nil {
			log.Printf("[Dashboard] Error adding comment to #%d: %v", issueNum, cmtErr)
		}

		if s.projectNumber > 0 {
			s.gh.MoveItemToColumn(s.projectNumber, issueNum, "Backlog")
		}

		log.Printf("[Dashboard] ✗ Merge conflict on #%d — PR closed, reset to backlog", issueNum)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.recordStep(issueNum, "merged", "PR merged successfully")

	if err := s.gh.RemoveLabel(issueNum, "awaiting-approval"); err != nil {
		log.Printf("[Dashboard] Error removing awaiting-approval label from #%d: %v", issueNum, err)
	}

	if s.projectNumber > 0 {
		s.gh.MoveItemToColumn(s.projectNumber, issueNum, "Done")
	}

	s.recordStep(issueNum, "done", "Moved to Done")

	log.Printf("[Dashboard] ✓ Approved & merged #%d", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	t, ok := s.tmpls[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	execName := "layout"
	if name == "workers.html" {
		execName = "workers.html"
	}
	if err := t.ExecuteTemplate(w, execName, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) renderFragment(w http.ResponseWriter, name string, data any) {
	t, ok := s.tmpls[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "content", data); err != nil {
		log.Printf("[Dashboard] Error rendering fragment %s: %v", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func toWorkerCard(info worker.WorkerInfo) workerCard {
	return workerCard{
		ID:      info.ID,
		Status:  string(info.Status),
		TaskID:  info.TaskID,
		Task:    info.TaskTitle,
		Stage:   info.Stage,
		Elapsed: formatDuration(info.Elapsed),
	}
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func placeholderWorkers() []workerCard {
	return []workerCard{
		{ID: "worker-1", Status: "working", TaskID: 3, Task: "Implement auth service", Stage: "coding", Elapsed: "2m 15s"},
		{ID: "worker-2", Status: "idle"},
		{ID: "worker-3", Status: "working", TaskID: 7, Task: "Add unit tests", Stage: "testing", Elapsed: "45s"},
	}
}

// LLMRequestTimeout is the timeout for LLM API requests
const LLMRequestTimeout = 60 * time.Second

func (s *Server) handleCurrentTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.orchestrator == nil {
		if err := json.NewEncoder(w).Encode(map[string]any{"active": false}); err != nil {
			log.Printf("[Dashboard] Error encoding JSON: %v", err)
		}
		return
	}
	task := s.orchestrator.CurrentTask()
	if task == nil {
		if err := json.NewEncoder(w).Encode(map[string]any{"active": false}); err != nil {
			log.Printf("[Dashboard] Error encoding JSON: %v", err)
		}
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]any{
		"active":       true,
		"issue_number": task.Issue.Number,
		"issue_title":  task.Issue.Title,
		"status":       string(task.Status),
		"milestone":    task.Milestone,
		"branch":       task.Branch,
	}); err != nil {
		log.Printf("[Dashboard] Error encoding JSON: %v", err)
	}
}

func (s *Server) handleSprintStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.orchestrator == nil {
		if err := json.NewEncoder(w).Encode(map[string]any{"paused": true, "processing": false}); err != nil {
			log.Printf("[Dashboard] Error encoding JSON: %v", err)
		}
		return
	}
	resp := map[string]any{
		"paused":     s.orchestrator.IsPaused(),
		"processing": s.orchestrator.IsProcessing(),
	}
	if task := s.orchestrator.CurrentTask(); task != nil {
		resp["current_issue"] = task.Issue.Number
		resp["current_status"] = string(task.Status)
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[Dashboard] Error encoding JSON: %v", err)
	}
}

func (s *Server) handleSprintStart(w http.ResponseWriter, r *http.Request) {
	if s.orchestrator == nil {
		http.Error(w, "orchestrator not configured", http.StatusServiceUnavailable)
		return
	}
	s.orchestrator.Start()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleSprintPause(w http.ResponseWriter, r *http.Request) {
	if s.orchestrator == nil {
		http.Error(w, "orchestrator not configured", http.StatusServiceUnavailable)
		return
	}
	s.orchestrator.Pause()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

type taskDetailData struct {
	Active      string
	IssueNumber int
	IssueTitle  string
	Steps       []db.TaskStep
	IsActive    bool
	Status      string
}

func (s *Server) handleTaskDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	issueNum, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid issue number", http.StatusBadRequest)
		return
	}

	var steps []db.TaskStep
	if s.store != nil {
		var err error
		steps, err = s.store.GetSteps(issueNum)
		if err != nil {
			log.Printf("[Dashboard] Error getting steps for #%d: %v", issueNum, err)
			steps = nil
		}
	}

	isActive := false
	status := ""
	issueTitle := ""
	if s.orchestrator != nil {
		if task := s.orchestrator.CurrentTask(); task != nil && task.Issue.Number == issueNum {
			isActive = true
			status = string(task.Status)
			issueTitle = task.Issue.Title
		}
	}

	if issueTitle == "" && s.gh != nil {
		if issue, err := s.gh.GetIssue(issueNum); err == nil {
			issueTitle = issue.Title
		}
	}
	if issueTitle == "" {
		issueTitle = fmt.Sprintf("Issue #%d", issueNum)
	}

	data := taskDetailData{
		Active:      "task",
		IssueNumber: issueNum,
		IssueTitle:  issueTitle,
		Steps:       steps,
		IsActive:    isActive,
		Status:      status,
	}
	s.render(w, "task.html", data)
}

func (s *Server) handleTaskStream(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	issueNum, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid issue number", http.StatusBadRequest)
		return
	}

	if s.orchestrator == nil {
		http.Error(w, "orchestrator not configured", http.StatusServiceUnavailable)
		return
	}

	task := s.orchestrator.CurrentTask()
	if task == nil || task.Issue.Number != issueNum {
		http.Error(w, "task not active", http.StatusNotFound)
		return
	}

	sessionID := task.SessionID()
	if sessionID == "" {
		http.Error(w, "no active LLM session", http.StatusNotFound)
		return
	}

	opencodeURL := s.orchestrator.OpenCodeURL() + "/event"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, opencodeURL, nil)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "failed to connect to opencode", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]

		var evt struct {
			Type       string          `json:"type"`
			Properties json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}

		switch evt.Type {
		case "message.part.delta":
			var props struct {
				SessionID string `json:"sessionID"`
				Delta     string `json:"delta"`
				Field     string `json:"field"`
			}
			if err := json.Unmarshal(evt.Properties, &props); err != nil {
				continue
			}
			if props.SessionID != sessionID || props.Field != "text" {
				continue
			}
			deltaJSON, _ := json.Marshal(map[string]string{"delta": props.Delta})
			fmt.Fprintf(w, "data: %s\n\n", deltaJSON)
			flusher.Flush()

		case "session.status":
			var props struct {
				SessionID string `json:"sessionID"`
				Status    struct {
					Type string `json:"type"`
				} `json:"status"`
			}
			if err := json.Unmarshal(evt.Properties, &props); err != nil {
				continue
			}
			if props.SessionID == sessionID && props.Status.Type == "idle" {
				fmt.Fprintf(w, "data: {\"done\":true}\n\n")
				flusher.Flush()
				return
			}
		}
	}
}

// handleWizardSelectType processes the type selection and redirects to the idea step
func (s *Server) handleWizardSelectType(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	sessionID := r.FormValue("session_id")
	wizardType := r.FormValue("wizard_type")
	isPage := r.FormValue("page") == "1"

	// Validate wizard type
	if wizardType != "feature" && wizardType != "bug" {
		http.Error(w, "invalid wizard type: must be 'feature' or 'bug'", http.StatusBadRequest)
		return
	}

	// Get or create session
	var session *WizardSession
	if sessionID != "" {
		if existing, ok := s.wizardStore.Get(sessionID); ok {
			session = existing
			session.Type = WizardType(wizardType)
			session.UpdatedAt = time.Now()
		}
	}

	if session == nil {
		var err error
		session, err = s.wizardStore.Create(wizardType)
		if err != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}
	}

	// Render the idea input step
	data := struct {
		Type      string
		SessionID string
		IsPage    bool
		Step      int
	}{
		Type:      wizardType,
		SessionID: session.ID,
		IsPage:    isPage,
		Step:      2, // Idea step
	}

	s.renderFragment(w, "wizard_new.html", data)
}
func (s *Server) handleWizardNew(w http.ResponseWriter, r *http.Request) {
	// Get wizard type from query param
	wizardType := r.URL.Query().Get("type")

	// Check for page mode
	isPage := r.URL.Query().Get("page") == "1"

	// If no type is provided, show the type selector
	if wizardType == "" {
		// Check for existing session ID (for back navigation)
		sessionID := r.URL.Query().Get("session_id")
		var session *WizardSession

		if sessionID != "" {
			// Try to get existing session
			if existing, ok := s.wizardStore.Get(sessionID); ok {
				session = existing
			}
		}

		// Create new session if not found
		if session == nil {
			// Create a temporary session without type - type will be set later
			now := time.Now()
			session = &WizardSession{
				ID:          uuid.New().String(),
				CurrentStep: WizardStepNew,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			s.wizardStore.sessions[session.ID] = session
		}

		data := struct {
			SessionID string
			IsPage    bool
			Step      int
		}{
			SessionID: session.ID,
			IsPage:    isPage,
			Step:      1, // Type selection step
		}

		s.renderFragment(w, "wizard_type_select.html", data)
		return
	}

	// Validate wizard type if provided
	if wizardType != "feature" && wizardType != "bug" {
		http.Error(w, "invalid wizard type: must be 'feature' or 'bug'", http.StatusBadRequest)
		return
	}

	// Check for existing session ID (for back navigation)
	sessionID := r.URL.Query().Get("session_id")
	var session *WizardSession

	if sessionID != "" {
		// Try to get existing session
		if existing, ok := s.wizardStore.Get(sessionID); ok {
			session = existing
		}
	}

	// Create new session if not found
	if session == nil {
		var err error
		session, err = s.wizardStore.Create(wizardType)
		if err != nil {
			http.Error(w, "invalid wizard type", http.StatusBadRequest)
			return
		}
	}

	data := struct {
		Type      string
		SessionID string
		IsPage    bool
		Step      int
	}{
		Type:      wizardType,
		SessionID: session.ID,
		IsPage:    isPage,
		Step:      2, // Idea step
	}

	s.renderFragment(w, "wizard_new.html", data)
}

// handleWizardRefine sends the idea to LLM and returns refined description
func (s *Server) handleWizardRefine(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	sessionID := r.FormValue("session_id")
	idea := r.FormValue("idea")
	currentDesc := r.FormValue("current_description")

	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}

	// Check for page mode
	isPage := r.FormValue("page") == "1" || r.URL.Query().Get("page") == "1"

	// Use current_description if provided (re-refinement), otherwise use idea
	inputText := currentDesc
	if inputText == "" {
		inputText = idea
	}

	if inputText == "" {
		http.Error(w, "missing idea or current_description", http.StatusBadRequest)
		return
	}

	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusBadRequest)
		return
	}

	// Validate input length to prevent abuse
	const maxIdeaLength = 10000
	if len(inputText) > maxIdeaLength {
		http.Error(w, "input exceeds maximum length of 10000 characters", http.StatusBadRequest)
		return
	}

	// Store the idea using thread-safe setter (only if it's a new idea, not re-refinement)
	if idea != "" {
		session.SetIdeaText(idea)
	}

	// Parse do_breakdown checkbox (only for features)
	doBreakdown := r.FormValue("do_breakdown") == "1"
	if session.Type == WizardTypeFeature {
		session.SetSkipBreakdown(!doBreakdown) // Inverted: unchecked = skip breakdown
	} else {
		// Bugs never do breakdown (they don't have the checkbox)
		session.SetSkipBreakdown(true)
	}

	// Parse add_to_sprint checkbox (available for all ticket types in refine step)
	addToSprint := r.FormValue("add_to_sprint") == "1"
	session.SetAddToSprint(addToSprint)

	session.SetStep(WizardStepRefine)
	session.AddLog("user", inputText)

	// If no opencode client, return mock response for testing
	if s.oc == nil {
		mockRefined := "Refined: " + inputText + "\n\nThis feature would allow users to authenticate securely."
		session.SetRefinedDescription(mockRefined)
		session.AddLog("assistant", mockRefined)

		data := struct {
			SessionID          string
			Type               string
			RefinedDescription string
			IsPage             bool
			SkipBreakdown      bool
			SprintName         string
		}{
			SessionID:          session.ID,
			Type:               string(session.Type),
			RefinedDescription: mockRefined,
			IsPage:             isPage,
			SkipBreakdown:      session.SkipBreakdown,
			SprintName:         s.activeSprintName(),
		}

		s.renderFragment(w, "wizard_refine.html", data)
		return
	}

	// Create LLM session for refinement
	llmSession, err := s.oc.CreateSession("Wizard Refinement")
	if err != nil {
		log.Printf("[Wizard] Error creating LLM session: %v", err)
		session.AddLog("system", "Error: Failed to create LLM session - "+err.Error())
		s.renderError(w, "Failed to connect to AI service. Please try again.", session.ID, string(session.Type), isPage)
		return
	}
	defer func() {
		if err := s.oc.DeleteSession(llmSession.ID); err != nil {
			log.Printf("[Wizard] Error deleting LLM session %s: %v", llmSession.ID, err)
		}
	}()

	// Build refinement prompt with codebase context
	codebaseContext := GetCodebaseContext()
	prompt := BuildRefinementPrompt(session.Type, inputText, codebaseContext)
	session.AddLog("system", "Sending refinement request to LLM")

	// Send message to LLM with timeout
	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(s.wizardLLM)
	var output strings.Builder
	response, err := s.oc.SendMessageStream(ctx, llmSession.ID, prompt, model, &output)
	if err != nil {
		log.Printf("[Wizard] Error from LLM: %v", err)
		session.AddLog("system", "LLM error: "+err.Error())

		errorMsg := "Failed to refine idea. "
		if ctx.Err() == context.DeadlineExceeded {
			errorMsg += "The AI service timed out. Please try again with a shorter description."
		} else {
			errorMsg += "Please check your connection and try again."
		}

		s.renderError(w, errorMsg, session.ID, string(session.Type), isPage)
		return
	}

	// Extract refined description from response
	var refinedDesc string
	if len(response.Parts) > 0 {
		refinedDesc = strings.TrimSpace(response.Parts[0].Text)
	}

	// Validate that we got a non-empty response
	if refinedDesc == "" {
		log.Printf("[Wizard] LLM returned empty response for session %s", session.ID)
		session.AddLog("system", "Error: LLM returned empty response")
		s.renderError(w, "The AI returned an empty response. Please try again with a more detailed description.", session.ID, string(session.Type), isPage)
		return
	}

	session.SetRefinedDescription(refinedDesc)
	session.AddLog("assistant", refinedDesc)

	data := struct {
		SessionID          string
		Type               string
		RefinedDescription string
		IsPage             bool
		SkipBreakdown      bool
		SprintName         string
	}{
		SessionID:          session.ID,
		Type:               string(session.Type),
		RefinedDescription: refinedDesc,
		IsPage:             isPage,
		SkipBreakdown:      session.SkipBreakdown,
		SprintName:         s.activeSprintName(),
	}

	s.renderFragment(w, "wizard_refine.html", data)
}

// renderError renders an error message in the wizard modal
func (s *Server) renderError(w http.ResponseWriter, errorMsg, sessionID, wizardType string, isPage bool) {
	data := struct {
		SessionID string
		Type      string
		Error     string
		IsPage    bool
	}{
		SessionID: sessionID,
		Type:      wizardType,
		Error:     errorMsg,
		IsPage:    isPage,
	}

	w.WriteHeader(http.StatusOK) // Return 200 so HTMX displays the content
	s.renderFragment(w, "wizard_error.html", data)
}

// handleWizardBreakdown sends description to LLM and returns task list
func (s *Server) handleWizardBreakdown(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	sessionID := r.FormValue("session_id")
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}

	// Check for page mode
	isPage := r.FormValue("page") == "1" || r.URL.Query().Get("page") == "1"

	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusBadRequest)
		return
	}

	if session.RefinedDescription == "" {
		http.Error(w, "no refined description found", http.StatusBadRequest)
		return
	}

	session.SetStep(WizardStepBreakdown)
	session.AddLog("system", "Starting task breakdown")

	// If no opencode client, return mock tasks for testing
	if s.oc == nil {
		mockTasks := []WizardTask{
			{
				Title:       "Set up authentication database schema",
				Description: "Create tables for users, sessions, and credentials",
				Priority:    "high",
				Complexity:  "M",
			},
			{
				Title:       "Implement login form UI",
				Description: "Create HTML/CSS form with email and password fields",
				Priority:    "medium",
				Complexity:  "S",
			},
		}
		session.SetTasks(mockTasks)
		session.AddLog("assistant", "Generated 2 tasks")

		data := struct {
			SessionID  string
			Tasks      []WizardTask
			IsPage     bool
			SprintName string
		}{
			SessionID:  session.ID,
			Tasks:      mockTasks,
			IsPage:     isPage,
			SprintName: s.activeSprintName(),
		}

		s.renderFragment(w, "wizard_breakdown.html", data)
		return
	}

	// Create LLM session for breakdown
	llmSession, err := s.oc.CreateSession("Wizard Breakdown")
	if err != nil {
		log.Printf("[Wizard] Error creating LLM session: %v", err)
		http.Error(w, "failed to create LLM session", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := s.oc.DeleteSession(llmSession.ID); err != nil {
			log.Printf("[Wizard] Error deleting LLM session %s: %v", llmSession.ID, err)
		}
	}()

	// Build breakdown prompt with JSON schema requirement
	prompt := BuildBreakdownPrompt(session.Type, session.RefinedDescription)
	session.AddLog("system", "Sending breakdown request to LLM")

	// Send message to LLM with timeout
	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(s.wizardLLM)
	var output strings.Builder
	response, err := s.oc.SendMessageStream(ctx, llmSession.ID, prompt, model, &output)
	if err != nil {
		log.Printf("[Wizard] Error from LLM: %v", err)
		session.AddLog("system", "LLM error: "+err.Error())
		http.Error(w, "failed to break down tasks", http.StatusInternalServerError)
		return
	}

	// Parse JSON response into tasks
	var tasks []WizardTask
	if len(response.Parts) > 0 {
		tasks = parseTaskJSON(response.Parts[0].Text)
	}

	session.SetTasks(tasks)
	session.AddLog("assistant", fmt.Sprintf("Generated %d tasks", len(tasks)))

	data := struct {
		SessionID  string
		Tasks      []WizardTask
		IsPage     bool
		SprintName string
	}{
		SessionID:  session.ID,
		Tasks:      tasks,
		IsPage:     isPage,
		SprintName: s.activeSprintName(),
	}

	s.renderFragment(w, "wizard_breakdown.html", data)
}

// buildBreakdownPrompt creates the prompt for task breakdown
func buildBreakdownPrompt(wizardType WizardType, description string) string {
	return fmt.Sprintf(`You are a technical project manager breaking down work into GitHub issues.

%s description:
%s

Break this down into 3-7 specific, actionable tasks. For each task provide:
- title: concise task title (max 80 chars)
- description: detailed technical description
- priority: one of [low, medium, high, critical]
- complexity: one of [S, M, L, XL] (S=1-2 hours, M=half day, L=1-2 days, XL=3+ days)

Return ONLY a JSON array in this exact format:
[
  {
    "title": "Task title",
    "description": "Task description",
    "priority": "high",
    "complexity": "M"
  }
]

No markdown, no explanation, just the JSON array.`, wizardType, description)
}

// handleWizardCreate creates GitHub issues with epic + sub-task structure
func (s *Server) handleWizardCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	sessionID := r.FormValue("session_id")
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}

	// Check for page mode
	isPage := r.FormValue("page") == "1" || r.URL.Query().Get("page") == "1"

	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusBadRequest)
		return
	}

	// Read sprint assignment preference from form
	addToSprint := r.FormValue("add_to_sprint") == "1"
	session.SetAddToSprint(addToSprint)

	session.SetStep(WizardStepCreate)

	// If skipping breakdown, create a single issue instead of epic + sub-tasks
	if session.SkipBreakdown {
		session.AddLog("system", "Creating single issue (breakdown skipped)")
		s.handleWizardCreateSingle(w, r, session, isPage)
		return
	}

	// Only check for tasks if not skipping breakdown
	if len(session.Tasks) == 0 {
		http.Error(w, "no tasks to create", http.StatusBadRequest)
		return
	}

	session.AddLog("system", fmt.Sprintf("Creating epic + %d sub-tasks", len(session.Tasks)))

	// If no GitHub client, return mock confirmation for testing
	if s.gh == nil {
		mockTitle := session.IdeaText
		if mockTitle == "" {
			mockTitle = session.RefinedDescription
		}
		if len(mockTitle) > 200 {
			mockTitle = mockTitle[:197] + "..."
		}
		mockEpic := CreatedIssue{
			Number:  100,
			Title:   mockTitle,
			URL:     "https://github.com/test/issues/100",
			IsEpic:  true,
			Success: true,
		}
		mockSubTasks := []CreatedIssue{
			{Number: 101, Title: session.Tasks[0].Title, URL: "https://github.com/test/issues/101", IsEpic: false, Success: true},
			{Number: 102, Title: session.Tasks[1].Title, URL: "https://github.com/test/issues/102", IsEpic: false, Success: true},
		}
		session.SetCreatedIssues(append([]CreatedIssue{mockEpic}, mockSubTasks...))
		session.SetEpicNumber(100)
		session.AddLog("system", "Mock: Created epic #100 with 2 sub-tasks")

		data := struct {
			Epic          CreatedIssue
			SubTasks      []CreatedIssue
			HasErrors     bool
			IsPage        bool
			IsSingleIssue bool
		}{
			Epic:          mockEpic,
			SubTasks:      mockSubTasks,
			HasErrors:     false,
			IsPage:        isPage,
			IsSingleIssue: false,
		}

		s.wizardStore.Delete(sessionID)
		s.renderFragment(w, "wizard_create.html", data)
		return
	}

	// Step 1: Create Epic First (abort on failure)
	epicLabels := []string{"epic"}
	if session.Type == WizardTypeFeature {
		epicLabels = append(epicLabels, "enhancement")
	} else if session.Type == WizardTypeBug {
		epicLabels = append(epicLabels, "bug")
	}

	epicTitle := session.IdeaText
	if epicTitle == "" {
		epicTitle = session.RefinedDescription
	}
	// GitHub issue titles have a 256 character limit
	if len(epicTitle) > 200 {
		epicTitle = epicTitle[:197] + "..."
	}

	epicBody := fmt.Sprintf("## Summary\n\n%s\n\n## Sub-tasks\n\n*Sub-tasks will be linked here after creation.*",
		session.RefinedDescription)

	epicNum, err := s.gh.CreateIssue(epicTitle, epicBody, epicLabels)
	if err != nil {
		log.Printf("[Wizard] Error creating epic: %v", err)
		session.AddLog("system", fmt.Sprintf("Error creating epic: %v", err))
		http.Error(w, fmt.Sprintf("Failed to create epic: %v", err), http.StatusInternalServerError)
		return
	}

	session.SetEpicNumber(epicNum)
	epicIssue := CreatedIssue{
		Number:  epicNum,
		Title:   epicTitle,
		URL:     fmt.Sprintf("https://github.com/%s/issues/%d", s.gh.Repo, epicNum),
		IsEpic:  true,
		Success: true,
	}
	session.AddCreatedIssue(epicIssue)
	session.AddLog("system", fmt.Sprintf("Created epic #%d", epicNum))

	// Assign epic to active sprint if requested
	sprintName := s.activeSprintName()
	if addToSprint && sprintName != "" {
		if err := s.gh.SetMilestone(epicNum, sprintName); err != nil {
			log.Printf("[Wizard] Error assigning epic #%d to sprint %s: %v", epicNum, sprintName, err)
			session.AddLog("system", fmt.Sprintf("Warning: could not assign epic to sprint: %v", err))
		} else {
			session.AddLog("system", fmt.Sprintf("Assigned epic #%d to %s", epicNum, sprintName))
		}
	}

	// Step 2: Create Sub-tasks (continue on individual failures)
	var subTaskLinks []string
	for _, task := range session.Tasks {
		// Map priority to label
		priorityLabel := ""
		switch strings.ToLower(task.Priority) {
		case "critical", "high":
			priorityLabel = "priority:high"
		case "medium":
			priorityLabel = "priority:medium"
		case "low":
			priorityLabel = "priority:low"
		}

		// Map complexity to size label
		sizeLabel := ""
		switch strings.ToUpper(task.Complexity) {
		case "S":
			sizeLabel = "size:S"
		case "M":
			sizeLabel = "size:M"
		case "L":
			sizeLabel = "size:L"
		case "XL":
			sizeLabel = "size:XL"
		}

		// Build labels array
		labels := []string{"wizard"}
		if priorityLabel != "" {
			labels = append(labels, priorityLabel)
		}
		if sizeLabel != "" {
			labels = append(labels, sizeLabel)
		}

		// Build sub-task body with parent reference
		body := fmt.Sprintf("## Description\n\n%s\n\n---\n\n**Parent Epic:** #%d\n**Priority:** %s\n**Complexity:** %s",
			task.Description,
			epicNum,
			task.Priority,
			task.Complexity,
		)

		// Create the sub-task issue
		issueNum, err := s.gh.CreateIssue(task.Title, body, labels)
		if err != nil {
			log.Printf("[Wizard] Error creating sub-task %q: %v", task.Title, err)
			session.AddLog("system", fmt.Sprintf("Error creating sub-task %q: %v", task.Title, err))
			session.AddCreatedIssue(CreatedIssue{
				Title:   task.Title,
				IsEpic:  false,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}

		issueURL := fmt.Sprintf("https://github.com/%s/issues/%d", s.gh.Repo, issueNum)
		subTaskLinks = append(subTaskLinks, fmt.Sprintf("- #%d: %s", issueNum, task.Title))

		session.AddCreatedIssue(CreatedIssue{
			Number:  issueNum,
			Title:   task.Title,
			URL:     issueURL,
			IsEpic:  false,
			Success: true,
		})
		session.AddLog("system", fmt.Sprintf("Created sub-task #%d: %s", issueNum, task.Title))

		// Assign sub-task to active sprint if requested
		if addToSprint && sprintName != "" {
			if err := s.gh.SetMilestone(issueNum, sprintName); err != nil {
				log.Printf("[Wizard] Error assigning #%d to sprint %s: %v", issueNum, sprintName, err)
				session.AddLog("system", fmt.Sprintf("Warning: could not assign #%d to sprint: %v", issueNum, err))
			}
		}
	}

	// Step 3: Update Epic Body with sub-task links
	if len(subTaskLinks) > 0 {
		updatedEpicBody := fmt.Sprintf("## Summary\n\n%s\n\n## Sub-tasks\n\n%s",
			session.RefinedDescription,
			strings.Join(subTaskLinks, "\n"),
		)
		if err := s.gh.UpdateIssueBody(epicNum, updatedEpicBody); err != nil {
			log.Printf("[Wizard] Error updating epic body: %v", err)
			session.AddLog("system", fmt.Sprintf("Error updating epic body: %v", err))
		} else {
			session.AddLog("system", fmt.Sprintf("Updated epic #%d with %d sub-task links", epicNum, len(subTaskLinks)))
		}
	}

	// Prepare data for template
	var subTasks []CreatedIssue
	hasErrors := false
	for _, issue := range session.CreatedIssues {
		if !issue.IsEpic {
			subTasks = append(subTasks, issue)
			if !issue.Success {
				hasErrors = true
			}
		}
	}

	data := struct {
		Epic          CreatedIssue
		SubTasks      []CreatedIssue
		HasErrors     bool
		IsPage        bool
		IsSingleIssue bool
	}{
		Epic:          epicIssue,
		SubTasks:      subTasks,
		HasErrors:     hasErrors,
		IsPage:        isPage,
		IsSingleIssue: false,
	}

	// Clean up session after creation to free memory
	s.wizardStore.Delete(sessionID)

	s.renderFragment(w, "wizard_create.html", data)
}

// handleWizardCreateSingle creates a single GitHub issue (for small tasks/bugs without breakdown)
func (s *Server) handleWizardCreateSingle(w http.ResponseWriter, r *http.Request, session *WizardSession, isPage bool) {
	// Read sprint assignment preference from form
	addToSprint := r.FormValue("add_to_sprint") == "1"
	session.SetAddToSprint(addToSprint)

	// If no GitHub client, return mock confirmation for testing
	if s.gh == nil {
		mockTitle := session.IdeaText
		if mockTitle == "" {
			mockTitle = session.RefinedDescription
		}
		if len(mockTitle) > 200 {
			mockTitle = mockTitle[:197] + "..."
		}
		mockIssue := CreatedIssue{
			Number:  100,
			Title:   mockTitle,
			URL:     "https://github.com/test/issues/100",
			IsEpic:  false,
			Success: true,
		}
		session.SetCreatedIssues([]CreatedIssue{mockIssue})
		session.AddLog("system", "Mock: Created single issue #100")

		data := struct {
			Epic          CreatedIssue
			SubTasks      []CreatedIssue
			HasErrors     bool
			IsPage        bool
			IsSingleIssue bool
		}{
			Epic:          mockIssue,
			SubTasks:      []CreatedIssue{},
			HasErrors:     false,
			IsPage:        isPage,
			IsSingleIssue: true,
		}

		s.wizardStore.Delete(session.ID)
		s.renderFragment(w, "wizard_create.html", data)
		return
	}

	// Build labels for single issue
	labels := []string{"wizard"}
	if session.Type == WizardTypeFeature {
		labels = append(labels, "enhancement")
	} else if session.Type == WizardTypeBug {
		labels = append(labels, "bug")
	}

	// Build title from idea text (truncated to 200 chars)
	title := session.IdeaText
	if title == "" {
		title = session.RefinedDescription
	}
	if len(title) > 200 {
		title = title[:197] + "..."
	}

	// Create the single issue
	body := session.RefinedDescription
	issueNum, err := s.gh.CreateIssue(title, body, labels)
	if err != nil {
		log.Printf("[Wizard] Error creating single issue: %v", err)
		session.AddLog("system", fmt.Sprintf("Error creating single issue: %v", err))
		http.Error(w, fmt.Sprintf("Failed to create issue: %v", err), http.StatusInternalServerError)
		return
	}

	issue := CreatedIssue{
		Number:  issueNum,
		Title:   title,
		URL:     fmt.Sprintf("https://github.com/%s/issues/%d", s.gh.Repo, issueNum),
		IsEpic:  false,
		Success: true,
	}
	session.AddCreatedIssue(issue)
	session.AddLog("system", fmt.Sprintf("Created single issue #%d", issueNum))

	// Assign to active sprint if requested
	sprintName := s.activeSprintName()
	if addToSprint && sprintName != "" {
		if err := s.gh.SetMilestone(issueNum, sprintName); err != nil {
			log.Printf("[Wizard] Error assigning #%d to sprint %s: %v", issueNum, sprintName, err)
			session.AddLog("system", fmt.Sprintf("Warning: could not assign to sprint: %v", err))
		} else {
			session.AddLog("system", fmt.Sprintf("Assigned #%d to %s", issueNum, sprintName))
		}
	}

	data := struct {
		Epic          CreatedIssue
		SubTasks      []CreatedIssue
		HasErrors     bool
		IsPage        bool
		IsSingleIssue bool
	}{
		Epic:          issue,
		SubTasks:      []CreatedIssue{},
		HasErrors:     false,
		IsPage:        isPage,
		IsSingleIssue: true,
	}

	// Clean up session after creation to free memory
	s.wizardStore.Delete(session.ID)

	s.renderFragment(w, "wizard_create.html", data)
}

// handleWizardLogs returns current LLM log entries for polling
func (s *Server) handleWizardLogs(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		http.Error(w, "missing session ID", http.StatusBadRequest)
		return
	}

	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Check if the session has moved past the expected step
	// If so, return 204 to stop HTMX polling
	expectedStep := r.Header.Get("X-Expected-Step")
	if expectedStep != "" && string(session.CurrentStep) != expectedStep {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	logs := session.GetLogs()

	data := struct {
		Logs []LLMLogEntry
	}{
		Logs: logs,
	}

	s.renderFragment(w, "wizard_logs.html", data)
}

// handleWizardPage returns the full wizard page (not modal)
func (s *Server) handleWizardPage(w http.ResponseWriter, r *http.Request) {
	// Get wizard type from query param
	wizardType := r.URL.Query().Get("type")

	// If no type is provided, show the type selector page
	if wizardType == "" {
		// Create a temporary session without type
		now := time.Now()
		session := &WizardSession{
			ID:          uuid.New().String(),
			CurrentStep: WizardStepNew,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		s.wizardStore.sessions[session.ID] = session

		data := struct {
			Active            string
			Type              string
			SessionID         string
			CurrentStep       int
			IsPage            bool
			ShowBreakdownStep bool
		}{
			Active:            "wizard",
			Type:              "",
			SessionID:         session.ID,
			CurrentStep:       1, // Type selection step
			IsPage:            true,
			ShowBreakdownStep: false,
		}

		s.render(w, "wizard_page.html", data)
		return
	}

	// Validate wizard type if provided
	if wizardType != "feature" && wizardType != "bug" {
		http.Error(w, "invalid wizard type: must be 'feature' or 'bug'", http.StatusBadRequest)
		return
	}

	// Create new session
	session, err := s.wizardStore.Create(wizardType)
	if err != nil {
		http.Error(w, "invalid wizard type", http.StatusBadRequest)
		return
	}

	data := struct {
		Active            string
		Type              string
		SessionID         string
		CurrentStep       int
		IsPage            bool
		ShowBreakdownStep bool
	}{
		Active:            "wizard",
		Type:              wizardType,
		SessionID:         session.ID,
		CurrentStep:       2, // Idea step
		IsPage:            true,
		ShowBreakdownStep: wizardType == "feature" && !session.SkipBreakdown,
	}

	s.render(w, "wizard_page.html", data)
}

// handleWizardModal returns the full modal shell with step 1 loaded
func (s *Server) handleWizardModal(w http.ResponseWriter, r *http.Request) {
	// Get wizard type from query param
	wizardType := r.URL.Query().Get("type")

	// If no type is provided, show the type selector
	if wizardType == "" {
		// Create a temporary session without type
		now := time.Now()
		session := &WizardSession{
			ID:          uuid.New().String(),
			CurrentStep: WizardStepNew,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		s.wizardStore.sessions[session.ID] = session

		data := struct {
			Type              string
			SessionID         string
			CurrentStep       int
			ShowBreakdownStep bool
		}{
			Type:              "",
			SessionID:         session.ID,
			CurrentStep:       1, // Type selection step
			ShowBreakdownStep: false,
		}

		s.renderFragment(w, "wizard_modal.html", data)
		return
	}

	// Validate wizard type if provided
	if wizardType != "feature" && wizardType != "bug" {
		http.Error(w, "invalid wizard type: must be 'feature' or 'bug'", http.StatusBadRequest)
		return
	}

	// Create new session
	session, err := s.wizardStore.Create(wizardType)
	if err != nil {
		http.Error(w, "invalid wizard type", http.StatusBadRequest)
		return
	}

	data := struct {
		Type              string
		SessionID         string
		CurrentStep       int
		ShowBreakdownStep bool
	}{
		Type:              wizardType,
		SessionID:         session.ID,
		CurrentStep:       2, // Idea step
		ShowBreakdownStep: wizardType == "feature" && !session.SkipBreakdown,
	}

	s.renderFragment(w, "wizard_modal.html", data)
}

// handleWizardCancel clears the wizard session and returns empty response
func (s *Server) handleWizardCancel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	sessionID := r.FormValue("session_id")
	if sessionID != "" {
		s.wizardStore.Delete(sessionID)
	}
	w.WriteHeader(http.StatusOK)
}
