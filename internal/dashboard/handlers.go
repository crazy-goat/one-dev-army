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

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

type taskCard struct {
	ID       int
	Title    string
	Status   string
	Worker   string
	Assignee string
	Labels   []string
	PRURL    string
	IsMerged bool
}

type boardData struct {
	Active         string
	SprintName     string
	Paused         bool
	Processing     bool
	CanCloseSprint bool
	CurrentIssue   string
	Blocked        []taskCard
	Backlog        []taskCard
	Plan           []taskCard
	Code           []taskCard
	AIReview       []taskCard
	Approve        []taskCard
	Merge          []taskCard
	Done           []taskCard
	Failed         []taskCard
}

func placeholderBoard() boardData {
	return boardData{
		Active: "board",
		Backlog: []taskCard{
			{ID: 1, Title: "Set up CI pipeline", Status: "backlog"},
			{ID: 2, Title: "Add logging middleware", Status: "backlog"},
		},
		Plan: []taskCard{
			{ID: 3, Title: "Design auth service architecture", Status: "plan", Worker: "worker-1"},
		},
		Code: []taskCard{
			{ID: 4, Title: "Implement auth service", Status: "code", Worker: "worker-2"},
		},
		AIReview: []taskCard{
			{ID: 5, Title: "Database migrations", Status: "ai_review"},
		},
		Approve: []taskCard{},
		Done: []taskCard{
			{ID: 6, Title: "Project skeleton", Status: "done"},
		},
		Blocked: []taskCard{
			{ID: 7, Title: "Deploy to staging", Status: "blocked"},
		},
	}
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	data := s.buildBoardData(r)
	s.render(w, "board.html", data)
}

func (s *Server) handleBoardData(w http.ResponseWriter, r *http.Request) {
	data := s.buildBoardData(r)
	s.renderTemplateBlock(w, "board.html", "board-columns", data)
}

func (s *Server) buildBoardData(r *http.Request) boardData {
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

	// If no GitHub client, no store, or no active milestone, return empty board
	if s.gh == nil || s.store == nil || s.gh.GetActiveMilestone() == nil {
		return data
	}

	milestone := s.gh.GetActiveMilestone().Title

	// Fetch issues from the cache instead of GitHub API
	issues, err := s.store.GetIssuesCacheByMilestone(milestone)
	if err != nil {
		log.Printf("[Dashboard] Error fetching cached issues for milestone %s: %v", milestone, err)
		return data
	}
	log.Printf("[Dashboard] Found %d cached issues in milestone %s", len(issues), milestone)

	// Infer status from issue labels and build task cards
	for _, issue := range issues {
		col := inferColumnFromIssue(issue)
		s.addCardToColumn(&data, col, issue)
	}

	// Check if sprint can be closed: all tasks in Done/Failed columns and not processing
	if !data.Processing &&
		len(data.Blocked) == 0 &&
		len(data.Backlog) == 0 &&
		len(data.Plan) == 0 &&
		len(data.Code) == 0 &&
		len(data.AIReview) == 0 &&
		len(data.Approve) == 0 &&
		len(data.Merge) == 0 {
		data.CanCloseSprint = true
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
	labels := issue.GetLabelNames()
	labelSet := make(map[string]bool)
	for _, l := range labels {
		labelSet[strings.ToLower(l)] = true
	}

	// Priority order matches state-machine.md Column Mapping.
	// Legacy bare labels kept for backward compatibility with existing issues.
	if labelSet["stage:blocked"] || labelSet["blocked"] {
		return "Blocked"
	}
	if labelSet["stage:failed"] || labelSet["failed"] {
		return "Failed"
	}
	if labelSet["stage:merging"] {
		return "Merge"
	}
	if labelSet["stage:awaiting-approval"] || labelSet["awaiting-approval"] {
		return "Approve"
	}
	if labelSet["stage:create-pr"] {
		return "AI Review" // Create PR is part of AI Review column
	}
	if labelSet["stage:code-review"] {
		return "AI Review"
	}
	if labelSet["stage:coding"] || labelSet["stage:testing"] || labelSet["in-progress"] {
		return "Code"
	}
	if labelSet["stage:analysis"] || labelSet["stage:planning"] {
		return "Plan"
	}

	if strings.EqualFold(issue.State, "CLOSED") {
		return "Done"
	}

	return "Backlog"
}

func (s *Server) addCardToColumn(data *boardData, col string, issue github.Issue) {
	card := taskCard{
		ID:       issue.Number,
		Title:    issue.Title,
		Status:   col,
		Assignee: issue.GetAssignee(),
		Labels:   issue.GetLabelNames(),
		IsMerged: issue.PRMerged,
	}

	switch col {
	case "Backlog":
		data.Backlog = append(data.Backlog, card)
	case "Plan":
		data.Plan = append(data.Plan, card)
	case "Code":
		data.Code = append(data.Code, card)
	case "AI Review":
		data.AIReview = append(data.AIReview, card)
	case "Approve":
		if s.store != nil {
			if prURL, err := s.store.GetStepResponse(issue.Number, "create-pr"); err == nil && prURL != "" {
				card.PRURL = prURL
			}
		}
		data.Approve = append(data.Approve, card)
	case "Merge":
		data.Merge = append(data.Merge, card)
	case "Done":
		data.Done = append(data.Done, card)
	case "Blocked":
		data.Blocked = append(data.Blocked, card)
	case "Failed":
		data.Failed = append(data.Failed, card)
	}
}

func (s *Server) handleAddEpic(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if s.gh != nil {
		m, err := s.gh.GetOldestOpenMilestone()
		if err != nil {
			log.Printf("[Dashboard] Sync error: %v", err)
		} else {
			s.gh.SetActiveMilestone(m)
			if s.syncService != nil {
				s.syncService.SetActiveMilestone(m.Title)
				s.syncService.SyncNow()
			}
			if m != nil {
				log.Printf("[Dashboard] Synced active milestone: %s", m.Title)
			} else {
				log.Printf("[Dashboard] Synced: no active milestone")
			}
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleManualSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.syncService == nil {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "sync service not configured"}); err != nil {
			log.Printf("[Dashboard] Error encoding JSON: %v", err)
		}
		return
	}

	if err := s.syncService.SyncNow(); err != nil {
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()}); err != nil {
			log.Printf("[Dashboard] Error encoding JSON: %v", err)
		}
		return
	}

	// Broadcast sync start message via WebSocket hub
	if s.hub != nil {
		s.hub.BroadcastSyncComplete(0)
	}

	if err := json.NewEncoder(w).Encode(map[string]any{"success": true}); err != nil {
		log.Printf("[Dashboard] Error encoding JSON: %v", err)
	}
}

func (s *Server) handlePlanSprint(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Set stage label to "Approve" using stage manager
	_, err := s.stageManager.ChangeStage(issueNum, "Approve", ReasonManualApprove, "dashboard")
	if err != nil {
		log.Printf("[Dashboard] Error setting Approve label on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	log.Printf("[Dashboard] Approved #%d — moved to Approve column", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Set stage label to "Backlog" (removes all stage labels) using stage manager
	_, err := s.stageManager.ChangeStage(issueNum, "Backlog", ReasonManualReject, "dashboard")
	if err != nil {
		log.Printf("[Dashboard] Error setting Backlog stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	log.Printf("[Dashboard] Rejected #%d — moved to Backlog", issueNum)
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

	// Set stage label to "Code" (removes failed label and adds coding label) using stage manager
	_, err := s.stageManager.ChangeStage(issueNum, "Code", ReasonManualRetry, "dashboard")
	if err != nil {
		log.Printf("[Dashboard] Error setting Code stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	log.Printf("[Dashboard] Retry #%d — moved to Code column", issueNum)
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

	// Set stage label to "Backlog" (removes all stage labels) using stage manager
	_, err := s.stageManager.ChangeStage(issueNum, "Backlog", ReasonManualRetry, "dashboard")
	if err != nil {
		log.Printf("[Dashboard] Error setting Backlog stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Clear DB steps
	if s.store != nil {
		if err := s.store.DeleteSteps(issueNum); err != nil {
			log.Printf("[Dashboard] Error deleting steps for #%d: %v", issueNum, err)
		}
	}

	log.Printf("[Dashboard] Retry fresh #%d — PR closed, steps cleared, moved to Backlog", issueNum)
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

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	_, err := s.stageManager.ChangeStage(issueNum, "Blocked", ReasonManualBlock, "dashboard")
	if err != nil {
		log.Printf("[Dashboard] Error setting Blocked stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	log.Printf("[Dashboard] Blocked #%d", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	_, err := s.stageManager.ChangeStage(issueNum, "Backlog", ReasonManualUnblock, "dashboard")
	if err != nil {
		log.Printf("[Dashboard] Error setting Backlog stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	log.Printf("[Dashboard] Unblocked #%d — moved to Backlog", issueNum)
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

	s.recordStep(issueNum, "declined", reason)

	// Set stage label to "Code" (removes awaiting-approval and adds coding label)
	_, err := s.stageManager.ChangeStage(issueNum, "Code", ReasonManualDecline, "dashboard")
	if err != nil {
		log.Printf("[Dashboard] Error setting Code stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
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

	// Transition to Merge stage before attempting merge
	_, mergeStageErr := s.stageManager.ChangeStage(issueNum, "Merge", ReasonManualMergeApproved, "dashboard")
	if mergeStageErr != nil {
		log.Printf("[Dashboard] Error setting Merge stage on #%d: %v", issueNum, mergeStageErr)
	}

	log.Printf("[Dashboard] Approving & merging PR for #%d (branch: %s)", issueNum, branch)
	if err := s.gh.MergePR(branch); err != nil {
		log.Printf("[Dashboard] ✗ Merge failed for #%d (likely conflict): %v", issueNum, err)
		s.recordStep(issueNum, "merge-failed", err.Error())

		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[Dashboard] Error closing PR for #%d: %v", issueNum, closeErr)
		}

		// Set stage label to "Failed" on merge failure
		_, labelErr := s.stageManager.ChangeStage(issueNum, "Failed", ReasonManualMergeApproved, "dashboard")
		if labelErr != nil {
			log.Printf("[Dashboard] Error setting Failed stage on #%d: %v", issueNum, labelErr)
		}

		if s.store != nil {
			if delErr := s.store.DeleteSteps(issueNum); delErr != nil {
				log.Printf("[Dashboard] Error deleting steps for #%d: %v", issueNum, delErr)
			}
		}

		comment := fmt.Sprintf("Merge failed (likely conflict). PR closed, task moved to Failed.\n\nError: %s", err.Error())
		if cmtErr := s.gh.AddComment(issueNum, comment); cmtErr != nil {
			log.Printf("[Dashboard] Error adding comment to #%d: %v", issueNum, cmtErr)
		}

		log.Printf("[Dashboard] ✗ Merge conflict on #%d — PR closed, moved to Failed", issueNum)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.recordStep(issueNum, "merged", "PR merged successfully")

	// Set stage label to "Done" on successful merge
	_, err = s.stageManager.ChangeStage(issueNum, "Done", ReasonManualMergeApproved, "dashboard")
	if err != nil {
		log.Printf("[Dashboard] Error setting Done stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
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
	s.renderTemplateBlock(w, name, "content", data)
}

func (s *Server) renderTemplateBlock(w http.ResponseWriter, name string, block string, data any) {
	t, ok := s.tmpls[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, block, data); err != nil {
		log.Printf("[Dashboard] Error rendering block %s from %s: %v", block, name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
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

// LLMRequestTimeout is the timeout for LLM API requests
const LLMRequestTimeout = 3 * time.Minute

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

func (s *Server) handleSprintClose(w http.ResponseWriter, r *http.Request) {
	// Validate orchestrator is not processing
	if s.orchestrator != nil && s.orchestrator.IsProcessing() {
		http.Error(w, "cannot close sprint while processing tasks", http.StatusConflict)
		return
	}

	// Get active milestone
	if s.gh == nil || s.gh.GetActiveMilestone() == nil {
		http.Error(w, "no active milestone", http.StatusBadRequest)
		return
	}

	milestone := s.gh.GetActiveMilestone()

	// Close the milestone via GitHub API
	if err := s.gh.CloseMilestone(milestone.Number); err != nil {
		log.Printf("[Dashboard] Error closing milestone %s: %v", milestone.Title, err)
		http.Error(w, fmt.Sprintf("failed to close milestone: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Dashboard] Closed milestone: %s", milestone.Title)

	// Auto-create a new sprint to ensure continuous sprint coverage
	newSprintTitle, err := s.gh.CreateNextSprint(milestone.Title)
	if err != nil {
		log.Printf("[Dashboard] Error creating next sprint after closing %s: %v", milestone.Title, err)
		http.Error(w, fmt.Sprintf("failed to create next sprint: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Dashboard] Created new sprint: %s", newSprintTitle)

	// Reload milestone state to get the newly created sprint
	newMilestone, err := s.gh.GetOldestOpenMilestone()
	if err != nil {
		log.Printf("[Dashboard] Error reloading milestones after closing %s: %v", milestone.Title, err)
		http.Error(w, fmt.Sprintf("failed to reload milestones: %v", err), http.StatusInternalServerError)
		return
	}

	// Validate that we have a new active sprint
	if newMilestone == nil {
		log.Printf("[Dashboard] No open milestone found after closing %s", milestone.Title)
		http.Error(w, "no active sprint available after closing", http.StatusInternalServerError)
		return
	}

	// Update GitHub client with the new active milestone
	s.gh.SetActiveMilestone(newMilestone)
	log.Printf("[Dashboard] Set new active milestone: %s", newMilestone.Title)

	// Update sync service with the new milestone title
	if s.syncService != nil {
		s.syncService.SetActiveMilestone(newMilestone.Title)
		log.Printf("[Dashboard] Updated sync service with new milestone: %s", newMilestone.Title)
	}

	// Trigger a sync to refresh cached data with the new sprint
	if s.syncService != nil {
		if err := s.syncService.SyncNow(); err != nil {
			log.Printf("[Dashboard] Warning: failed to trigger sync after sprint close: %v", err)
			// Don't fail the operation if sync trigger fails
		} else {
			log.Printf("[Dashboard] Triggered sync to refresh data for new sprint: %s", newMilestone.Title)
		}
	}

	// Redirect to board
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

// handleWizardNew returns the initial wizard modal form
func (s *Server) handleWizardNew(w http.ResponseWriter, r *http.Request) {
	// Get wizard type from query param
	wizardType := r.URL.Query().Get("type")

	// Check for page mode
	isPage := r.URL.Query().Get("page") == "1"

	// Check for existing session ID (for back navigation)
	sessionID := r.URL.Query().Get("session_id")
	var session *WizardSession

	if sessionID != "" {
		// Try to get existing session
		if existing, ok := s.wizardStore.Get(sessionID); ok {
			session = existing
		}
	}

	// If no type param or invalid type, and no session, show type selector
	isValidType := wizardType == "feature" || wizardType == "bug"
	needsTypeSelection := (wizardType == "" || !isValidType) && session == nil

	// Create new session if valid type is provided and no session exists
	if !needsTypeSelection && session == nil {
		var err error
		session, err = s.wizardStore.Create(wizardType)
		if err != nil {
			http.Error(w, "invalid wizard type", http.StatusBadRequest)
			return
		}
	}

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
	language := r.FormValue("language") // NEW: Read language parameter

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

	// Store language preference if provided
	if language != "" {
		session.SetLanguage(language)
	}

	// Parse add_to_sprint checkbox
	addToSprint := r.FormValue("add_to_sprint") == "1"
	session.SetAddToSprint(addToSprint)

	// In new unified flow, always skip breakdown (step removed)
	session.SetSkipBreakdown(true)

	session.SetStep(WizardStepRefine)
	session.AddLog("user", inputText)

	// If no opencode client, return mock response for testing
	if s.oc == nil {
		mockTitle := "[Feature] Add user authentication system"
		if session.Type == WizardTypeBug {
			mockTitle = "[Bug] Fix authentication issue"
		}
		mockPlanning := "## Description\n\nAdd user authentication to the system.\n\n## Tasks\n\n1. Create auth service in `internal/auth/service.go`\n2. Add user storage in `internal/db/users.go`\n3. Add login endpoint handler\n4. Write tests\n\n## Files to Modify\n\n- `internal/auth/service.go` - New file: authentication logic\n- `internal/db/users.go` - New file: user storage\n\n## Acceptance Criteria\n\n- Users can log in with username and password\n- Sessions are maintained across requests\n- Invalid credentials are rejected"
		session.SetTechnicalPlanning(mockPlanning)
		session.SetGeneratedTitle(mockTitle)
		session.SetPriority("medium")
		session.SetComplexity("M")
		session.AddLog("assistant", "Generated title: "+mockTitle+"\nPriority: medium | Complexity: M\n\n"+mockPlanning)

		data := struct {
			SessionID          string
			Type               string
			Title              string
			TechnicalPlanning  string
			Priority           string
			Complexity         string
			IsPage             bool
			SkipBreakdown      bool
			SprintName         string
			CurrentStep        int
			ShowBreakdownStep  bool
			NeedsTypeSelection bool
		}{
			SessionID:          session.ID,
			Type:               string(session.Type),
			Title:              session.GetFinalTitle(),
			TechnicalPlanning:  mockPlanning,
			Priority:           session.Priority,
			Complexity:         session.Complexity,
			IsPage:             isPage,
			SkipBreakdown:      true, // Always skip breakdown in new flow
			SprintName:         s.activeSprintName(),
			CurrentStep:        2,     // Now step 2 is Review
			ShowBreakdownStep:  false, // No more breakdown step
			NeedsTypeSelection: false,
		}

		s.renderFragment(w, "wizard_refine.html", data)
		return
	}

	// Create LLM session
	llmSession, err := s.oc.CreateSession("Wizard Issue Generation")
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

	codebaseContext := GetCodebaseContext()
	prompt := BuildIssueGenerationPrompt(session.Type, inputText, codebaseContext, session.Language)
	session.AddLog("system", "Sending issue generation request to LLM (language: "+session.Language+")")

	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(s.wizardLLM)
	var result GeneratedIssue
	err = s.oc.SendMessageStructured(ctx, llmSession.ID, prompt, model, GeneratedIssueSchema, &result)
	if err != nil {
		log.Printf("[Wizard] Error from LLM: %v", err)
		session.AddLog("system", "LLM error: "+err.Error())
		errorMsg := "Failed to generate issue. "
		if ctx.Err() == context.DeadlineExceeded {
			errorMsg += "The AI service timed out. Please try again with a shorter description."
		} else {
			errorMsg += "Please check your connection and try again."
		}
		s.renderError(w, errorMsg, session.ID, string(session.Type), isPage)
		return
	}

	if result.Description == "" {
		log.Printf("[Wizard] LLM returned empty description for session %s", session.ID)
		session.AddLog("system", "Error: LLM returned empty description")
		s.renderError(w, "The AI returned an empty response. Please try again with a more detailed description.", session.ID, string(session.Type), isPage)
		return
	}

	// Ensure title has proper prefix
	if result.Title != "" {
		if !strings.HasPrefix(result.Title, "[Feature]") && !strings.HasPrefix(result.Title, "[Bug]") {
			if session.Type == WizardTypeBug {
				result.Title = "[Bug] " + result.Title
			} else {
				result.Title = "[Feature] " + result.Title
			}
		}
		if len(result.Title) > 80 {
			result.Title = result.Title[:77] + "..."
		}
	} else {
		if session.Type == WizardTypeBug {
			result.Title = "[Bug] Fix issue"
		} else {
			result.Title = "[Feature] New feature"
		}
	}

	session.SetTechnicalPlanning(result.Description)
	session.SetGeneratedTitle(result.Title)
	session.SetPriority(result.Priority)
	session.SetComplexity(result.Complexity)
	session.AddLog("assistant", fmt.Sprintf("Generated title: %s\nPriority: %s | Complexity: %s\n\n%s",
		result.Title, result.Priority, result.Complexity, result.Description))

	data := struct {
		SessionID          string
		Type               string
		Title              string
		TechnicalPlanning  string
		Priority           string
		Complexity         string
		IsPage             bool
		SkipBreakdown      bool
		SprintName         string
		CurrentStep        int
		ShowBreakdownStep  bool
		NeedsTypeSelection bool
	}{
		SessionID:          session.ID,
		Type:               string(session.Type),
		Title:              session.GetFinalTitle(),
		TechnicalPlanning:  session.TechnicalPlanning,
		Priority:           session.Priority,
		Complexity:         session.Complexity,
		IsPage:             isPage,
		SkipBreakdown:      true, // Always skip breakdown in new flow
		SprintName:         s.activeSprintName(),
		CurrentStep:        2,     // Now step 2 is Review
		ShowBreakdownStep:  false, // No more breakdown step
		NeedsTypeSelection: false,
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

	// Always create a single issue in the new unified flow
	session.AddLog("system", "Creating single issue from technical planning")
	s.handleWizardCreateSingle(w, r, session, isPage)
}

// handleWizardCreateSingle creates a single GitHub issue (for the unified flow)
func (s *Server) handleWizardCreateSingle(w http.ResponseWriter, r *http.Request, session *WizardSession, isPage bool) {
	// Read sprint assignment preference from form
	addToSprint := r.FormValue("add_to_sprint") == "1"
	session.SetAddToSprint(addToSprint)

	// Read custom title from form (user may have edited it on the review page)
	customTitle := r.FormValue("issue_title")
	if customTitle != "" {
		session.SetCustomTitle(customTitle)
		session.SetUseCustomTitle(true)
	}

	// If no GitHub client, return mock confirmation for testing
	if s.gh == nil {
		mockTitle := session.GetFinalTitle()
		if mockTitle == "" {
			if session.Type == WizardTypeBug {
				mockTitle = "[Bug] Fix issue"
			} else {
				mockTitle = "[Feature] New feature"
			}
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
			Epic               CreatedIssue
			SubTasks           []CreatedIssue
			HasErrors          bool
			IsPage             bool
			IsSingleIssue      bool
			CurrentStep        int
			ShowBreakdownStep  bool
			NeedsTypeSelection bool
			Type               string
		}{
			Epic:               mockIssue,
			SubTasks:           []CreatedIssue{},
			HasErrors:          false,
			IsPage:             isPage,
			IsSingleIssue:      true,
			CurrentStep:        3, // Step 3 is Create in new 3-step flow
			ShowBreakdownStep:  false,
			NeedsTypeSelection: false,
			Type:               string(session.Type),
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

	// Add LLM-estimated priority and complexity labels
	gi := GeneratedIssue{Priority: session.Priority, Complexity: session.Complexity}
	if label := gi.PriorityLabel(); label != "" {
		labels = append(labels, label)
	}
	if label := gi.ComplexityLabel(); label != "" {
		labels = append(labels, label)
	}

	// Get title from session (either custom or generated)
	title := session.GetFinalTitle()
	if title == "" {
		// Simple fallback
		if session.Type == WizardTypeBug {
			title = "[Bug] Fix issue"
		} else {
			title = "[Feature] New feature"
		}
	}

	// Validate title length (GitHub limit is 256, but we enforce 80)
	if len(title) > 80 {
		title = title[:77] + "..."
	}

	// Create the single issue
	body := session.TechnicalPlanning
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

	// Trigger immediate sync to make new ticket appear on dashboard
	// Sync failure must not block the creation flow
	if s.syncService != nil {
		go func() {
			if err := s.syncService.SyncNow(); err != nil {
				log.Printf("[Wizard] Sync error after issue creation: %v", err)
			}
		}()
	}

	data := struct {
		Epic               CreatedIssue
		SubTasks           []CreatedIssue
		HasErrors          bool
		IsPage             bool
		IsSingleIssue      bool
		CurrentStep        int
		ShowBreakdownStep  bool
		NeedsTypeSelection bool
		Type               string
	}{
		Epic:               issue,
		SubTasks:           []CreatedIssue{},
		HasErrors:          false,
		IsPage:             isPage,
		IsSingleIssue:      true,
		CurrentStep:        3, // Step 3 is Create in new 3-step flow
		ShowBreakdownStep:  false,
		NeedsTypeSelection: false,
		Type:               string(session.Type),
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

	// Check for existing session ID (for back navigation)
	sessionID := r.URL.Query().Get("session_id")
	var session *WizardSession

	if sessionID != "" {
		// Try to get existing session
		if existing, ok := s.wizardStore.Get(sessionID); ok {
			session = existing
		}
	}

	// If no type param or invalid type, and no session, show type selector
	isValidType := wizardType == "feature" || wizardType == "bug"
	needsTypeSelection := (wizardType == "" || !isValidType) && session == nil

	// Create new session if valid type is provided and no session exists
	if !needsTypeSelection && session == nil {
		var err error
		session, err = s.wizardStore.Create(wizardType)
		if err != nil {
			http.Error(w, "invalid wizard type", http.StatusBadRequest)
			return
		}
	}

	data := struct {
		Active             string
		Type               string
		SessionID          string
		CurrentStep        int
		IsPage             bool
		ShowBreakdownStep  bool
		NeedsTypeSelection bool
	}{
		Active:             "wizard",
		Type:               wizardType,
		SessionID:          "",
		CurrentStep:        1,
		IsPage:             true,
		ShowBreakdownStep:  false,
		NeedsTypeSelection: needsTypeSelection,
	}

	if session != nil {
		data.SessionID = session.ID
		data.Type = string(session.Type)
		data.ShowBreakdownStep = session.Type == WizardTypeFeature && !session.SkipBreakdown
	}

	s.render(w, "wizard_page.html", data)
}

// handleWizardModal returns the full modal shell with step 1 loaded
func (s *Server) handleWizardModal(w http.ResponseWriter, r *http.Request) {
	// Get wizard type from query param
	wizardType := r.URL.Query().Get("type")

	// Check for existing session ID (for back navigation)
	sessionID := r.URL.Query().Get("session_id")
	var session *WizardSession

	if sessionID != "" {
		// Try to get existing session
		if existing, ok := s.wizardStore.Get(sessionID); ok {
			session = existing
		}
	}

	// If no type param or invalid type, and no session, show type selector
	isValidType := wizardType == "feature" || wizardType == "bug"
	needsTypeSelection := (wizardType == "" || !isValidType) && session == nil

	// Create new session if valid type is provided and no session exists
	if !needsTypeSelection && session == nil {
		var err error
		session, err = s.wizardStore.Create(wizardType)
		if err != nil {
			http.Error(w, "invalid wizard type", http.StatusBadRequest)
			return
		}
	}

	data := struct {
		Type               string
		SessionID          string
		CurrentStep        int
		ShowBreakdownStep  bool
		NeedsTypeSelection bool
		IsPage             bool
	}{
		Type:               wizardType,
		SessionID:          "",
		CurrentStep:        1,
		ShowBreakdownStep:  false,
		NeedsTypeSelection: needsTypeSelection,
		IsPage:             false, // Modal is never page mode
	}

	if session != nil {
		data.SessionID = session.ID
		data.Type = string(session.Type)
		data.ShowBreakdownStep = session.Type == WizardTypeFeature && !session.SkipBreakdown
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

// handleWizardSelectType handles type selection and creates a session
func (s *Server) handleWizardSelectType(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	wizardType := r.FormValue("wizard_type")
	if wizardType != "feature" && wizardType != "bug" {
		http.Error(w, "invalid wizard type: must be 'feature' or 'bug'", http.StatusBadRequest)
		return
	}

	// Create new session with selected type
	session, err := s.wizardStore.Create(wizardType)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Check for page mode
	isPage := r.FormValue("page") == "1" || r.URL.Query().Get("page") == "1"

	// Build template data for idea step
	data := struct {
		Type               string
		SessionID          string
		IsPage             bool
		CurrentStep        int
		ShowBreakdownStep  bool
		NeedsTypeSelection bool
		Language           string
	}{
		Type:               wizardType,
		SessionID:          session.ID,
		IsPage:             isPage,
		CurrentStep:        1, // Now on Idea step (step 1 in 4-step flow)
		ShowBreakdownStep:  session.Type == WizardTypeFeature && !session.SkipBreakdown,
		NeedsTypeSelection: false,
		Language:           session.Language,
	}

	s.renderFragment(w, "wizard_new.html", data)
}

// handleRateLimit returns the current GitHub API rate limit status as HTML fragment
func (s *Server) handleRateLimit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if s.rateLimitService == nil {
		w.Write([]byte(`<span class="rate-limit-unknown">GitHub API: Not configured</span>`))
		return
	}

	summary := s.rateLimitService.GetSummary()
	if summary == nil {
		w.Write([]byte(`<span class="rate-limit-unknown">GitHub API: Loading...</span>`))
		return
	}

	// Calculate worst percentage
	worstPercentage := summary.GetWorstPercentage()
	color := summary.GetWorstColorCSS()
	worstLimit := summary.GetWorstLimit()

	// Build the compressed indicator HTML with tooltip
	var html strings.Builder

	// Warning icon if there's an error but we have cached data
	warningIcon := ""
	if summary.Error != "" && !summary.UpdatedAt.IsZero() {
		warningIcon = " ⚠"
	}

	// Compressed indicator showing worst percentage
	html.WriteString(fmt.Sprintf(
		`<div class="rate-limit-compressed" style="color: %s;" title="Click to refresh" hx-post="/api/rate-limit/refresh" hx-swap="outerHTML">`,
		color,
	))
	html.WriteString(fmt.Sprintf(
		`<span class="rate-limit-percentage">GitHub API usage: %.0f%%%s</span>`,
		worstPercentage,
		warningIcon,
	))

	// Add tooltip panel with detailed breakdown
	html.WriteString(`<div class="rate-limit-tooltip">`)
	html.WriteString(`<div class="rate-limit-tooltip-header">GitHub API Rate Limits</div>`)
	html.WriteString(`<div class="rate-limit-tooltip-content">`)

	// Core API
	if summary.Core != nil {
		corePercentage := summary.Core.GetUsagePercentage()
		coreColor := GetColorCSSByPercentage(corePercentage)
		html.WriteString(fmt.Sprintf(
			`<div class="rate-limit-row"><span class="rate-limit-label">REST API:</span><span class="rate-limit-value" style="color: %s">%d/%d (%.0f%%)</span><span class="rate-limit-reset">%s</span></div>`,
			coreColor,
			summary.Core.Remaining,
			summary.Core.Limit,
			corePercentage,
			summary.Core.GetResetTimeFormatted(),
		))
	}

	// GraphQL API
	if summary.GraphQL != nil {
		graphqlPercentage := summary.GraphQL.GetUsagePercentage()
		graphqlColor := GetColorCSSByPercentage(graphqlPercentage)
		html.WriteString(fmt.Sprintf(
			`<div class="rate-limit-row"><span class="rate-limit-label">GraphQL:</span><span class="rate-limit-value" style="color: %s">%d/%d (%.0f%%)</span><span class="rate-limit-reset">%s</span></div>`,
			graphqlColor,
			summary.GraphQL.Remaining,
			summary.GraphQL.Limit,
			graphqlPercentage,
			summary.GraphQL.GetResetTimeFormatted(),
		))
	}

	// Search API
	if summary.Search != nil {
		searchPercentage := summary.Search.GetUsagePercentage()
		searchColor := GetColorCSSByPercentage(searchPercentage)
		html.WriteString(fmt.Sprintf(
			`<div class="rate-limit-row"><span class="rate-limit-label">Search:</span><span class="rate-limit-value" style="color: %s">%d/%d (%.0f%%)</span><span class="rate-limit-reset">%s</span></div>`,
			searchColor,
			summary.Search.Remaining,
			summary.Search.Limit,
			searchPercentage,
			summary.Search.GetResetTimeFormatted(),
		))
	}

	html.WriteString(`</div>`) // Close tooltip-content

	// Show which limit is the worst
	if worstLimit != nil {
		html.WriteString(fmt.Sprintf(
			`<div class="rate-limit-tooltip-footer">Worst: %s (%.0f%%)</div>`,
			worstLimit.Name,
			worstPercentage,
		))
	}

	html.WriteString(`</div>`) // Close tooltip
	html.WriteString(`</div>`) // Close rate-limit-compressed

	w.Write([]byte(html.String()))
}

// handleRateLimitRefresh triggers a manual refresh of the rate limit data
func (s *Server) handleRateLimitRefresh(w http.ResponseWriter, r *http.Request) {
	if s.rateLimitService != nil {
		s.rateLimitService.Refresh()
		// Small delay to allow the refresh to complete
		time.Sleep(100 * time.Millisecond)
	}
	// Return the updated status
	s.handleRateLimit(w, r)
}

// settingsData holds the data for the settings template
type settingsData struct {
	Active            string
	Config            config.LLMConfig
	ForceStrongStages string
	Success           bool
	Errors            []string
	AvailableModels   []opencode.ProviderModel
}

// handleSettings renders the LLM configuration settings page
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	// Load current config
	cfg, err := config.Load(s.rootDir)
	if err != nil {
		log.Printf("[Dashboard] Error loading config: %v", err)
		// Use default config if load fails
		defaultCfg := config.DefaultLLMConfig()
		cfg = &config.Config{LLM: defaultCfg}
	}

	// Build comma-separated list of forced strong stages
	forceStrongStages := strings.Join(cfg.LLM.RoutingRules.ForceStrongForStages, ", ")

	data := settingsData{
		Active:            "settings",
		Config:            cfg.LLM,
		ForceStrongStages: forceStrongStages,
		AvailableModels:   s.modelsCache,
	}

	s.render(w, "llm-config.html", data)
}

// handleSaveSettings processes the settings form submission
func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderSettingsWithErrors(w, r, []string{"Failed to parse form data"})
		return
	}

	// Load existing config
	cfg, err := config.Load(s.rootDir)
	if err != nil {
		log.Printf("[Dashboard] Error loading config: %v", err)
		cfg = &config.Config{LLM: config.DefaultLLMConfig()}
	}

	// Parse and validate form data
	var errors []string

	// Parse Development models
	cfg.LLM.Development.Strong.Provider = r.FormValue("development_strong_provider")
	cfg.LLM.Development.Strong.Model = r.FormValue("development_strong_model")
	cfg.LLM.Development.Strong.APIKey = r.FormValue("development_strong_api_key")
	cfg.LLM.Development.Strong.BaseURL = r.FormValue("development_strong_base_url")

	cfg.LLM.Development.Weak.Provider = r.FormValue("development_weak_provider")
	cfg.LLM.Development.Weak.Model = r.FormValue("development_weak_model")
	cfg.LLM.Development.Weak.APIKey = r.FormValue("development_weak_api_key")
	cfg.LLM.Development.Weak.BaseURL = r.FormValue("development_weak_base_url")

	// Parse Planning models
	cfg.LLM.Planning.Strong.Provider = r.FormValue("planning_strong_provider")
	cfg.LLM.Planning.Strong.Model = r.FormValue("planning_strong_model")
	cfg.LLM.Planning.Strong.APIKey = r.FormValue("planning_strong_api_key")
	cfg.LLM.Planning.Strong.BaseURL = r.FormValue("planning_strong_base_url")

	cfg.LLM.Planning.Weak.Provider = r.FormValue("planning_weak_provider")
	cfg.LLM.Planning.Weak.Model = r.FormValue("planning_weak_model")
	cfg.LLM.Planning.Weak.APIKey = r.FormValue("planning_weak_api_key")
	cfg.LLM.Planning.Weak.BaseURL = r.FormValue("planning_weak_base_url")

	// Parse Orchestration models
	cfg.LLM.Orchestration.Strong.Provider = r.FormValue("orchestration_strong_provider")
	cfg.LLM.Orchestration.Strong.Model = r.FormValue("orchestration_strong_model")
	cfg.LLM.Orchestration.Strong.APIKey = r.FormValue("orchestration_strong_api_key")
	cfg.LLM.Orchestration.Strong.BaseURL = r.FormValue("orchestration_strong_base_url")

	cfg.LLM.Orchestration.Weak.Provider = r.FormValue("orchestration_weak_provider")
	cfg.LLM.Orchestration.Weak.Model = r.FormValue("orchestration_weak_model")
	cfg.LLM.Orchestration.Weak.APIKey = r.FormValue("orchestration_weak_api_key")
	cfg.LLM.Orchestration.Weak.BaseURL = r.FormValue("orchestration_weak_base_url")

	// Parse Setup models
	cfg.LLM.Setup.Strong.Provider = r.FormValue("setup_strong_provider")
	cfg.LLM.Setup.Strong.Model = r.FormValue("setup_strong_model")
	cfg.LLM.Setup.Strong.APIKey = r.FormValue("setup_strong_api_key")
	cfg.LLM.Setup.Strong.BaseURL = r.FormValue("setup_strong_base_url")

	cfg.LLM.Setup.Weak.Provider = r.FormValue("setup_weak_provider")
	cfg.LLM.Setup.Weak.Model = r.FormValue("setup_weak_model")
	cfg.LLM.Setup.Weak.APIKey = r.FormValue("setup_weak_api_key")
	cfg.LLM.Setup.Weak.BaseURL = r.FormValue("setup_weak_base_url")

	// Validate required fields
	categories := []struct {
		name   string
		strong config.ModelConfig
		weak   config.ModelConfig
	}{
		{"Development", cfg.LLM.Development.Strong, cfg.LLM.Development.Weak},
		{"Planning", cfg.LLM.Planning.Strong, cfg.LLM.Planning.Weak},
		{"Orchestration", cfg.LLM.Orchestration.Strong, cfg.LLM.Orchestration.Weak},
		{"Setup", cfg.LLM.Setup.Strong, cfg.LLM.Setup.Weak},
	}

	for _, cat := range categories {
		if cat.strong.Provider == "" {
			errors = append(errors, fmt.Sprintf("%s Strong: Provider is required", cat.name))
		}
		if cat.strong.Model == "" {
			errors = append(errors, fmt.Sprintf("%s Strong: Model is required", cat.name))
		}
		if cat.weak.Provider == "" {
			errors = append(errors, fmt.Sprintf("%s Weak: Provider is required", cat.name))
		}
		if cat.weak.Model == "" {
			errors = append(errors, fmt.Sprintf("%s Weak: Model is required", cat.name))
		}
	}

	// Validate model selections against available models
	if len(s.modelsCache) > 0 {
		modelValidations := []struct {
			name     string
			provider string
			model    string
		}{
			{"Development Strong", cfg.LLM.Development.Strong.Provider, cfg.LLM.Development.Strong.Model},
			{"Development Weak", cfg.LLM.Development.Weak.Provider, cfg.LLM.Development.Weak.Model},
			{"Planning Strong", cfg.LLM.Planning.Strong.Provider, cfg.LLM.Planning.Strong.Model},
			{"Planning Weak", cfg.LLM.Planning.Weak.Provider, cfg.LLM.Planning.Weak.Model},
			{"Orchestration Strong", cfg.LLM.Orchestration.Strong.Provider, cfg.LLM.Orchestration.Strong.Model},
			{"Orchestration Weak", cfg.LLM.Orchestration.Weak.Provider, cfg.LLM.Orchestration.Weak.Model},
			{"Setup Strong", cfg.LLM.Setup.Strong.Provider, cfg.LLM.Setup.Strong.Model},
			{"Setup Weak", cfg.LLM.Setup.Weak.Provider, cfg.LLM.Setup.Weak.Model},
		}

		for _, mv := range modelValidations {
			if mv.provider != "" && mv.model != "" && !s.validateModelSelection(mv.provider, mv.model) {
				errors = append(errors, fmt.Sprintf("%s: Invalid model '%s/%s' - not found in available models", mv.name, mv.provider, mv.model))
			}
		}
	}

	// Parse routing thresholds
	codeSizeThreshold, err := strconv.Atoi(r.FormValue("routing_code_size_threshold"))
	if err != nil || codeSizeThreshold < 1 {
		errors = append(errors, "Code Size Threshold must be a positive integer")
	} else {
		cfg.LLM.RoutingRules.ComplexityThresholds.CodeSizeThreshold = codeSizeThreshold
	}

	highComplexityThreshold, err := strconv.Atoi(r.FormValue("routing_high_complexity_threshold"))
	if err != nil || highComplexityThreshold < 1 {
		errors = append(errors, "High Complexity Threshold must be a positive integer")
	} else {
		cfg.LLM.RoutingRules.ComplexityThresholds.HighComplexityThreshold = highComplexityThreshold
	}

	fileCountThreshold, err := strconv.Atoi(r.FormValue("routing_file_count_threshold"))
	if err != nil || fileCountThreshold < 1 {
		errors = append(errors, "File Count Threshold must be a positive integer")
	} else {
		cfg.LLM.RoutingRules.ComplexityThresholds.FileCountThreshold = fileCountThreshold
	}

	// Parse forced strong stages
	forceStrongStagesStr := r.FormValue("routing_force_strong_stages")
	if forceStrongStagesStr != "" {
		// Split by comma and trim whitespace
		stages := strings.Split(forceStrongStagesStr, ",")
		cfg.LLM.RoutingRules.ForceStrongForStages = make([]string, 0, len(stages))
		for _, stage := range stages {
			stage = strings.TrimSpace(stage)
			if stage != "" {
				cfg.LLM.RoutingRules.ForceStrongForStages = append(cfg.LLM.RoutingRules.ForceStrongForStages, stage)
			}
		}
	} else {
		cfg.LLM.RoutingRules.ForceStrongForStages = []string{}
	}

	// If there are validation errors, re-render the form with errors
	if len(errors) > 0 {
		s.renderSettingsWithErrors(w, r, errors)
		return
	}

	// Save the config
	if err := config.SaveConfig(s.rootDir, cfg); err != nil {
		log.Printf("[Dashboard] Error saving config: %v", err)
		s.renderSettingsWithErrors(w, r, []string{fmt.Sprintf("Failed to save configuration: %v", err)})
		return
	}

	log.Printf("[Dashboard] LLM configuration saved successfully")

	// Re-render with success message
	forceStrongStages := strings.Join(cfg.LLM.RoutingRules.ForceStrongForStages, ", ")
	data := settingsData{
		Active:            "settings",
		Config:            cfg.LLM,
		ForceStrongStages: forceStrongStages,
		Success:           true,
		AvailableModels:   s.modelsCache,
	}

	s.render(w, "llm-config.html", data)
}

// renderSettingsWithErrors renders the settings page with validation errors
func (s *Server) renderSettingsWithErrors(w http.ResponseWriter, r *http.Request, errors []string) {
	// Load current config to populate the form
	cfg, err := config.Load(s.rootDir)
	if err != nil {
		log.Printf("[Dashboard] Error loading config: %v", err)
		cfg = &config.Config{LLM: config.DefaultLLMConfig()}
	}

	// Override with form values to preserve user input
	// This is a simplified approach - in production, you'd want to preserve all form values
	forceStrongStages := r.FormValue("routing_force_strong_stages")

	data := settingsData{
		Active:            "settings",
		Config:            cfg.LLM,
		ForceStrongStages: forceStrongStages,
		Errors:            errors,
		AvailableModels:   s.modelsCache,
	}

	s.render(w, "llm-config.html", data)
}

// validateModelSelection checks if a model selection is valid against the cached models
func (s *Server) validateModelSelection(provider, model string) bool {
	if len(s.modelsCache) == 0 {
		return true // Skip validation if cache is empty (API unavailable)
	}

	for _, m := range s.modelsCache {
		if m.ProviderID == provider && m.ID == model {
			return true
		}
	}
	return false
}
