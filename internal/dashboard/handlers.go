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

func (s *Server) handleDecline(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	reason := r.FormValue("reason")

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

	log.Printf("[Dashboard] Approving & merging PR for #%d (branch: %s)", issueNum, branch)
	if err := s.gh.MergePR(branch); err != nil {
		log.Printf("[Dashboard] ✗ Merge failed for #%d (likely conflict): %v", issueNum, err)

		// Close PR and delete branch
		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[Dashboard] Error closing PR for #%d: %v", issueNum, closeErr)
		}

		// Remove labels, clear steps — ticket goes back to backlog for fresh start
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

	if err := s.gh.RemoveLabel(issueNum, "awaiting-approval"); err != nil {
		log.Printf("[Dashboard] Error removing awaiting-approval label from #%d: %v", issueNum, err)
	}

	if s.projectNumber > 0 {
		s.gh.MoveItemToColumn(s.projectNumber, issueNum, "Done")
	}

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

	steps, err := s.store.GetSteps(issueNum)
	if err != nil {
		log.Printf("[Dashboard] Error getting steps for #%d: %v", issueNum, err)
		steps = nil
	}

	isActive := false
	status := ""
	issueTitle := fmt.Sprintf("#%d", issueNum)
	if s.orchestrator != nil {
		if task := s.orchestrator.CurrentTask(); task != nil && task.Issue.Number == issueNum {
			isActive = true
			status = string(task.Status)
			issueTitle = task.Issue.Title
		}
	}

	if issueTitle == fmt.Sprintf("#%d", issueNum) && len(steps) > 0 {
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

// handleWizardNew returns the initial wizard modal form
func (s *Server) handleWizardNew(w http.ResponseWriter, r *http.Request) {
	// Get wizard type from query param (default to feature)
	wizardType := r.URL.Query().Get("type")

	// Validate wizard type
	if wizardType == "" {
		wizardType = "feature"
	} else if wizardType != "feature" && wizardType != "bug" {
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
	}{
		Type:      wizardType,
		SessionID: session.ID,
	}

	s.render(w, "wizard_new.html", data)
}

// handleWizardRefine sends the idea to LLM and returns refined description
func (s *Server) handleWizardRefine(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	sessionID := r.FormValue("session_id")
	idea := r.FormValue("idea")

	if sessionID == "" || idea == "" {
		http.Error(w, "missing session_id or idea", http.StatusBadRequest)
		return
	}

	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusBadRequest)
		return
	}

	// Validate idea length to prevent abuse
	const maxIdeaLength = 10000
	if len(idea) > maxIdeaLength {
		http.Error(w, "idea exceeds maximum length of 10000 characters", http.StatusBadRequest)
		return
	}

	// Store the idea using thread-safe setter
	session.SetIdeaText(idea)
	session.SetStep(WizardStepRefine)
	session.AddLog("user", idea)

	// If no opencode client, return mock response for testing
	if s.oc == nil {
		mockRefined := "Refined: " + idea + "\n\nThis feature would allow users to authenticate securely."
		session.SetRefinedDescription(mockRefined)
		session.AddLog("assistant", mockRefined)

		data := struct {
			SessionID          string
			Type               string
			RefinedDescription string
		}{
			SessionID:          session.ID,
			Type:               string(session.Type),
			RefinedDescription: mockRefined,
		}

		s.render(w, "wizard_refine.html", data)
		return
	}

	// Create LLM session for refinement
	llmSession, err := s.oc.CreateSession("Wizard Refinement")
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

	// Build refinement prompt with codebase context
	codebaseContext := GetCodebaseContext()
	prompt := BuildRefinementPrompt(session.Type, idea, codebaseContext)
	session.AddLog("system", "Sending refinement request to LLM")

	// Send message to LLM with timeout
	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(DefaultLLMModel)
	var output strings.Builder
	response, err := s.oc.SendMessageStream(ctx, llmSession.ID, prompt, model, &output)
	if err != nil {
		log.Printf("[Wizard] Error from LLM: %v", err)
		session.AddLog("system", "LLM error: "+err.Error())
		http.Error(w, "failed to refine idea", http.StatusInternalServerError)
		return
	}

	// Extract refined description from response
	var refinedDesc string
	if len(response.Parts) > 0 {
		refinedDesc = response.Parts[0].Text
	}

	session.SetRefinedDescription(refinedDesc)
	session.AddLog("assistant", refinedDesc)

	data := struct {
		SessionID          string
		Type               string
		RefinedDescription string
	}{
		SessionID:          session.ID,
		Type:               string(session.Type),
		RefinedDescription: refinedDesc,
	}

	s.render(w, "wizard_refine.html", data)
}

// handleWizardRefineAgain re-refines the existing description using LLM
func (s *Server) handleWizardRefineAgain(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data. Please try again.", http.StatusBadRequest)
		return
	}

	sessionID := r.FormValue("session_id")
	if sessionID == "" {
		http.Error(w, "Session ID is required.", http.StatusBadRequest)
		return
	}

	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		http.Error(w, "Session not found. Please start over.", http.StatusNotFound)
		return
	}

	// Get the current refined description to use as input
	currentDescription := session.RefinedDescription
	if currentDescription == "" {
		// Fall back to idea text if no refined description exists
		currentDescription = session.IdeaText
	}

	if currentDescription == "" {
		http.Error(w, "No description available to refine. Please go back and enter an idea.", http.StatusBadRequest)
		return
	}

	session.AddLog("user", "Requesting further refinement")

	// If no opencode client, return mock response for testing
	if s.oc == nil {
		mockRefined := "Further refined: " + currentDescription + "\n\nThis version includes additional clarity and technical details."
		session.SetRefinedDescription(mockRefined)
		session.AddLog("assistant", mockRefined)

		data := struct {
			SessionID          string
			Type               string
			RefinedDescription string
			Error              string
		}{
			SessionID:          session.ID,
			Type:               string(session.Type),
			RefinedDescription: mockRefined,
		}

		s.render(w, "wizard_refine.html", data)
		return
	}

	// Create LLM session for refinement
	llmSession, err := s.oc.CreateSession("Wizard Refinement - Iteration")
	if err != nil {
		log.Printf("[Wizard] Error creating LLM session: %v", err)
		http.Error(w, "Unable to connect to AI service. Please try again in a moment.", http.StatusServiceUnavailable)
		return
	}
	defer func() {
		if err := s.oc.DeleteSession(llmSession.ID); err != nil {
			log.Printf("[Wizard] Error deleting LLM session %s: %v", llmSession.ID, err)
		}
	}()

	// Build refinement prompt with codebase context
	codebaseContext := GetCodebaseContext()
	prompt := BuildRefinementPrompt(session.Type, currentDescription, codebaseContext)
	session.AddLog("system", "Sending refinement request to LLM (iteration)")

	// Send message to LLM with timeout
	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(DefaultLLMModel)
	var output strings.Builder
	response, err := s.oc.SendMessageStream(ctx, llmSession.ID, prompt, model, &output)
	if err != nil {
		log.Printf("[Wizard] Error from LLM: %v", err)
		session.AddLog("system", "LLM error: "+err.Error())

		// Determine appropriate error message based on error type
		statusCode := http.StatusInternalServerError
		errorMsg := "Failed to refine description. Please try again."

		if ctx.Err() == context.DeadlineExceeded {
			statusCode = http.StatusGatewayTimeout
			errorMsg = "The AI service is taking too long to respond. Please try again."
		}

		http.Error(w, errorMsg, statusCode)
		return
	}

	// Extract refined description from response
	var refinedDesc string
	if len(response.Parts) > 0 {
		refinedDesc = response.Parts[0].Text
	}

	session.SetRefinedDescription(refinedDesc)
	session.AddLog("assistant", refinedDesc)

	data := struct {
		SessionID          string
		Type               string
		RefinedDescription string
		Error              string
	}{
		SessionID:          session.ID,
		Type:               string(session.Type),
		RefinedDescription: refinedDesc,
	}

	s.render(w, "wizard_refine.html", data)
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
			SessionID string
			Tasks     []WizardTask
		}{
			SessionID: session.ID,
			Tasks:     mockTasks,
		}

		s.render(w, "wizard_breakdown.html", data)
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

	model := opencode.ParseModelRef(DefaultLLMModel)
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
		SessionID string
		Tasks     []WizardTask
	}{
		SessionID: session.ID,
		Tasks:     tasks,
	}

	s.render(w, "wizard_breakdown.html", data)
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

// handleWizardCreate creates GitHub issues and returns confirmation
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

	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusBadRequest)
		return
	}

	if len(session.Tasks) == 0 {
		http.Error(w, "no tasks to create", http.StatusBadRequest)
		return
	}

	session.SetStep(WizardStepCreate)
	session.AddLog("system", fmt.Sprintf("Creating %d GitHub issues", len(session.Tasks)))

	// If no GitHub client, return mock confirmation for testing
	if s.gh == nil {
		mockIssues := []struct {
			Number int
			Title  string
			URL    string
		}{
			{Number: 101, Title: session.Tasks[0].Title, URL: "https://github.com/test/issues/101"},
			{Number: 102, Title: session.Tasks[1].Title, URL: "https://github.com/test/issues/102"},
		}
		session.AddLog("system", "Mock: Created 2 issues")

		data := struct {
			CreatedIssues []struct {
				Number int
				Title  string
				URL    string
			}
		}{
			CreatedIssues: mockIssues,
		}

		// Clean up session after mock creation to free memory
		s.wizardStore.Delete(sessionID)

		s.render(w, "wizard_create.html", data)
		return
	}

	// Create GitHub issues for each task
	type createdIssue struct {
		Number int
		Title  string
		URL    string
	}
	var createdIssues []createdIssue

	for _, task := range session.Tasks {
		// Build issue body
		body := fmt.Sprintf("## Description\n\n%s\n\n## Priority\n%s\n\n## Complexity\n%s",
			task.Description,
			task.Priority,
			task.Complexity,
		)

		// Create the issue
		issueNum, err := s.gh.CreateIssue(task.Title, body, []string{"wizard"})
		if err != nil {
			log.Printf("[Wizard] Error creating issue for task %q: %v", task.Title, err)
			session.AddLog("system", fmt.Sprintf("Error creating issue: %v", err))
			continue
		}

		createdIssues = append(createdIssues, createdIssue{
			Number: issueNum,
			Title:  task.Title,
			URL:    fmt.Sprintf("https://github.com/%s/issues/%d", s.gh.Repo, issueNum),
		})

		session.AddLog("system", fmt.Sprintf("Created issue #%d: %s", issueNum, task.Title))
	}

	data := struct {
		CreatedIssues []createdIssue
	}{
		CreatedIssues: createdIssues,
	}

	// Clean up session after successful creation to free memory
	s.wizardStore.Delete(sessionID)

	s.render(w, "wizard_create.html", data)
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

	logs := session.GetLogs()

	data := struct {
		Logs []LLMLogEntry
	}{
		Logs: logs,
	}

	s.render(w, "wizard_logs.html", data)
}

// handleWizardModal returns the full modal shell with step 1 loaded
func (s *Server) handleWizardModal(w http.ResponseWriter, r *http.Request) {
	// Get wizard type from query param (default to feature)
	wizardType := r.URL.Query().Get("type")
	if wizardType != "bug" {
		wizardType = "feature"
	}

	// Create new session
	session, err := s.wizardStore.Create(wizardType)
	if err != nil {
		http.Error(w, "invalid wizard type", http.StatusBadRequest)
		return
	}

	data := struct {
		Type        string
		SessionID   string
		CurrentStep int
	}{
		Type:        wizardType,
		SessionID:   session.ID,
		CurrentStep: 1,
	}

	s.render(w, "wizard_modal.html", data)
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
