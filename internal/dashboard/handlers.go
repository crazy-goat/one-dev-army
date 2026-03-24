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
	"github.com/crazy-goat/one-dev-army/internal/mvp"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

const (
	columnAIReview      = "AI Review"
	defaultBugTitle     = "[Bug] Fix issue"
	defaultFeatureTitle = "[Feature] New feature"
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
	OpenCodePort   int
	WorkerCount    int
	SprintName     string
	Paused         bool
	Processing     bool
	CanCloseSprint bool
	CurrentIssue   string
	YoloMode       bool
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

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	data := s.buildBoardData(r)
	s.render(w, "board.html", data)
}

func (s *Server) handleBoardData(w http.ResponseWriter, r *http.Request) {
	data := s.buildBoardData(r)
	s.renderTemplateBlock(w, "board.html", "board-columns", data)
}

func (s *Server) buildBoardData(_ *http.Request) boardData {
	workerCount := 0
	if s.pool != nil {
		workerCount = len(s.pool())
	}

	// Load config to get yolo mode status
	yoloMode := false
	if s.rootDir != "" {
		if cfg, err := config.Load(s.rootDir); err == nil {
			yoloMode = cfg.YoloMode
		}
	}

	data := boardData{
		Active:       "board",
		OpenCodePort: s.webPort,
		WorkerCount:  workerCount,
		Paused:       true,
		YoloMode:     yoloMode,
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
		return columnAIReview // Create PR is part of AI Review column
	}
	if labelSet["stage:code-review"] {
		return columnAIReview
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

func (*Server) handleAddEpic(w http.ResponseWriter, r *http.Request) {
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
				if err := s.syncService.SyncNow(); err != nil {
					log.Printf("[Dashboard] Sync error: %v", err)
				}
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

func (s *Server) handleManualSync(w http.ResponseWriter, _ *http.Request) {
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

func (*Server) handlePlanSprint(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Set stage label via orchestrator
	err := s.orchestrator.ChangeStage(issueNum, github.StageApprove, github.ReasonManualApprove)
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
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Set stage label to backlog (removes all stage labels) via orchestrator
	err := s.orchestrator.ChangeStage(issueNum, github.StageBacklog, github.ReasonManualReject)
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
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Set stage label to coding (removes failed label and adds coding label) via orchestrator
	err := s.orchestrator.ChangeStage(issueNum, github.StageCode, github.ReasonManualRetry)
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
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
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

	// Set stage label to backlog (removes all stage labels) via orchestrator
	err := s.orchestrator.ChangeStage(issueNum, github.StageBacklog, github.ReasonManualRetryFresh)
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
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	err := s.orchestrator.ChangeStage(issueNum, github.StageBlocked, github.ReasonManualBlock)
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
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	err := s.orchestrator.ChangeStage(issueNum, github.StageBacklog, github.ReasonManualUnblock)
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
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	reason := r.FormValue("reason")
	s.recordStep(issueNum, "declined", reason)

	err := s.orchestrator.SendDecision(issueNum, mvp.UserDecision{Action: "decline", Reason: reason})
	if err != nil {
		log.Printf("[Dashboard] Error sending decline decision for #%d: %v — falling back to direct stage change", issueNum, err)
		// Fallback: direct stage change if worker not processing
		_ = s.orchestrator.ChangeStage(issueNum, github.StageCode, github.ReasonManualDecline)
		if reason != "" {
			comment := "**Declined** — sent back for fixes.\n\n" + reason
			_ = s.gh.AddComment(issueNum, comment)
		}
		if s.store != nil {
			_ = s.store.DeleteSteps(issueNum)
		}
	} else {
		log.Printf("[Dashboard] ✓ Sent decline decision for #%d", issueNum)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleApproveMerge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.recordStep(issueNum, "approved", "Manual approval granted")

	err := s.orchestrator.SendDecision(issueNum, mvp.UserDecision{Action: "approve"})
	if err != nil {
		log.Printf("[Dashboard] Error sending approve decision for #%d: %v — falling back to direct merge", issueNum, err)
		// Fallback: if worker is not processing (e.g. after restart), do direct merge
		s.handleDirectMerge(w, r, issueNum)
		return
	}

	log.Printf("[Dashboard] ✓ Sent approve decision for #%d", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleDirectMerge is a fallback for when the worker is not processing the issue
// (e.g. after ODA restart while in awaiting-approval state).
func (s *Server) handleDirectMerge(w http.ResponseWriter, r *http.Request, issueNum int) {
	if s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	branch, err := s.gh.FindPRBranch(issueNum)
	if err != nil {
		log.Printf("[Dashboard] Error finding PR for #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	mergeStageErr := s.orchestrator.ChangeStage(issueNum, github.StageMerge, github.ReasonManualMerge)
	if mergeStageErr != nil {
		log.Printf("[Dashboard] Error setting Merge stage on #%d: %v", issueNum, mergeStageErr)
	}

	log.Printf("[Dashboard] Direct merging PR for #%d (branch: %s)", issueNum, branch)
	if err := s.gh.MergePR(branch); err != nil {
		log.Printf("[Dashboard] ✗ Direct merge failed for #%d: %v", issueNum, err)
		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[Dashboard] Error closing PR for #%d: %v", issueNum, closeErr)
		}
		_ = s.orchestrator.ChangeStage(issueNum, github.StageFailed, github.ReasonManualMergeFailed)

		comment := "Merge failed (likely conflict). PR closed, task moved to Failed.\n\nError: " + err.Error()
		_ = s.gh.AddComment(issueNum, comment)

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	_ = s.orchestrator.ChangeStage(issueNum, github.StageDone, github.ReasonManualMerge)
	log.Printf("[Dashboard] ✓ Direct merged #%d (fallback)", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	t, ok := s.tmpls[name]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("[Dashboard] Template execution error: %v", err)
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
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

const LLMRequestTimeout = 3 * time.Minute

func (s *Server) handleCurrentTask(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleSprintStatus(w http.ResponseWriter, _ *http.Request) {
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
	Active       string
	OpenCodePort int
	WorkerCount  int
	IssueNumber  int
	IssueTitle   string
	Steps        []db.TaskStep
	IsActive     bool
	Status       string
	YoloMode     bool
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

	workerCount := 0
	if s.pool != nil {
		workerCount = len(s.pool())
	}

	// Load config to get yolo mode status
	yoloMode := false
	if s.rootDir != "" {
		if cfg, err := config.Load(s.rootDir); err == nil {
			yoloMode = cfg.YoloMode
		}
	}

	data := taskDetailData{
		Active:       "task",
		OpenCodePort: s.webPort,
		WorkerCount:  workerCount,
		IssueNumber:  issueNum,
		IssueTitle:   issueTitle,
		Steps:        steps,
		IsActive:     isActive,
		Status:       status,
		YoloMode:     yoloMode,
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// First, send all historical chat messages
	chatMessages := task.GetChatMessages()
	if len(chatMessages) > 0 {
		historyJSON, _ := json.Marshal(map[string]any{
			"history": chatMessages,
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", historyJSON)
		flusher.Flush()
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
	defer func() { _ = resp.Body.Close() }()

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
			_, _ = fmt.Fprintf(w, "data: %s\n\n", deltaJSON)
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
				_, _ = fmt.Fprintf(w, "data: {\"done\":true}\n\n")
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
			result.Title = defaultBugTitle
		} else {
			result.Title = defaultFeatureTitle
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
				mockTitle = defaultBugTitle
			} else {
				mockTitle = defaultFeatureTitle
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
	switch session.Type {
	case WizardTypeFeature:
		labels = append(labels, "enhancement")
	case WizardTypeBug:
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
			title = defaultBugTitle
		} else {
			title = defaultFeatureTitle
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

	workerCount := 0
	if s.pool != nil {
		workerCount = len(s.pool())
	}
	data := struct {
		Active             string
		OpenCodePort       int
		WorkerCount        int
		Type               string
		SessionID          string
		CurrentStep        int
		IsPage             bool
		ShowBreakdownStep  bool
		NeedsTypeSelection bool
	}{
		Active:             "wizard",
		OpenCodePort:       s.webPort,
		WorkerCount:        workerCount,
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
func (s *Server) handleRateLimit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if s.rateLimitService == nil {
		_, _ = w.Write([]byte(`<span class="rate-limit-unknown">GitHub API: Not configured</span>`))
		return
	}

	summary := s.rateLimitService.GetSummary()
	if summary == nil {
		_, _ = w.Write([]byte(`<span class="rate-limit-unknown">GitHub API: Loading...</span>`))
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
	fmt.Fprintf(&html,
		`<div class="rate-limit-compressed" style="color: %s;" title="Click to refresh" hx-post="/api/rate-limit/refresh" hx-swap="outerHTML">`,
		color,
	)
	fmt.Fprintf(&html,
		`<span class="rate-limit-percentage">GitHub API usage: %.0f%%%s</span>`,
		worstPercentage,
		warningIcon,
	)

	// Add tooltip panel with detailed breakdown
	html.WriteString(`<div class="rate-limit-tooltip">`)
	html.WriteString(`<div class="rate-limit-tooltip-header">GitHub API Rate Limits</div>`)
	html.WriteString(`<div class="rate-limit-tooltip-content">`)

	// Core API
	if summary.Core != nil {
		corePercentage := summary.Core.GetUsagePercentage()
		coreColor := GetColorCSSByPercentage(corePercentage)
		fmt.Fprintf(&html,
			`<div class="rate-limit-row"><span class="rate-limit-label">REST API:</span><span class="rate-limit-value" style="color: %s">%d/%d (%.0f%%)</span><span class="rate-limit-reset">%s</span></div>`,
			coreColor,
			summary.Core.Remaining,
			summary.Core.Limit,
			corePercentage,
			summary.Core.GetResetTimeFormatted(),
		)
	}

	// GraphQL API
	if summary.GraphQL != nil {
		graphqlPercentage := summary.GraphQL.GetUsagePercentage()
		graphqlColor := GetColorCSSByPercentage(graphqlPercentage)
		fmt.Fprintf(&html,
			`<div class="rate-limit-row"><span class="rate-limit-label">GraphQL:</span><span class="rate-limit-value" style="color: %s">%d/%d (%.0f%%)</span><span class="rate-limit-reset">%s</span></div>`,
			graphqlColor,
			summary.GraphQL.Remaining,
			summary.GraphQL.Limit,
			graphqlPercentage,
			summary.GraphQL.GetResetTimeFormatted(),
		)
	}

	// Search API
	if summary.Search != nil {
		searchPercentage := summary.Search.GetUsagePercentage()
		searchColor := GetColorCSSByPercentage(searchPercentage)
		fmt.Fprintf(&html,
			`<div class="rate-limit-row"><span class="rate-limit-label">Search:</span><span class="rate-limit-value" style="color: %s">%d/%d (%.0f%%)</span><span class="rate-limit-reset">%s</span></div>`,
			searchColor,
			summary.Search.Remaining,
			summary.Search.Limit,
			searchPercentage,
			summary.Search.GetResetTimeFormatted(),
		)
	}

	html.WriteString(`</div>`) // Close tooltip-content

	// Show which limit is the worst
	if worstLimit != nil {
		fmt.Fprintf(&html,
			`<div class="rate-limit-tooltip-footer">Worst: %s (%.0f%%)</div>`,
			worstLimit.Name,
			worstPercentage,
		)
	}

	html.WriteString(`</div>`) // Close tooltip
	html.WriteString(`</div>`) // Close rate-limit-compressed

	_, _ = w.Write([]byte(html.String()))
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
	OpenCodePort      int
	WorkerCount       int
	Config            config.LLMConfig
	YoloMode          bool
	ForceStrongStages string
	Success           bool
	Errors            []string
	AvailableModels   []opencode.ProviderModel
}

// handleSettings renders the LLM configuration settings page
func (s *Server) handleSettings(w http.ResponseWriter, _ *http.Request) {
	// Load current config with model validation and fallback
	availableModels := s.GetAvailableModelIDs()
	cfg, err := config.Load(s.rootDir, availableModels...)
	if err != nil {
		log.Printf("[Dashboard] Error loading config: %v", err)
		// Use default config if load fails
		defaultCfg := config.DefaultLLMConfig()
		cfg = &config.Config{LLM: defaultCfg}
	}

	// Build comma-separated list of forced strong stages
	forceStrongStages := strings.Join(cfg.LLM.RoutingRules.ForceStrongForStages, ", ") //nolint:staticcheck // deprecated but kept for backward compatibility

	workerCount := 0
	if s.pool != nil {
		workerCount = len(s.pool())
	}
	data := settingsData{
		Active:            "settings",
		OpenCodePort:      s.webPort,
		WorkerCount:       workerCount,
		Config:            cfg.LLM,
		YoloMode:          cfg.YoloMode,
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
	var warnings []string

	// Parse model selections for each mode (5 independent dropdowns)
	cfg.LLM.Setup.Model = r.FormValue("setup_model")
	cfg.LLM.Planning.Model = r.FormValue("planning_model")
	cfg.LLM.Orchestration.Model = r.FormValue("orchestration_model")
	cfg.LLM.Code.Model = r.FormValue("code_model")
	cfg.LLM.CodeHeavy.Model = r.FormValue("code_heavy_model")

	// Validate required fields - 5 single models instead of 4 strong/weak pairs
	modes := []struct {
		name  string
		model string
	}{
		{"Setup", cfg.LLM.Setup.Model},
		{"Planning", cfg.LLM.Planning.Model},
		{"Orchestration", cfg.LLM.Orchestration.Model},
		{"Code", cfg.LLM.Code.Model},
		{"Code Heavy", cfg.LLM.CodeHeavy.Model},
	}

	for _, mode := range modes {
		if mode.model == "" {
			errors = append(errors, mode.name+": Model is required")
		} else if !validateModelFormat(mode.model) {
			errors = append(errors, mode.name+": Model must be in 'provider/model' format (e.g., 'nexos-ai/Kimi K2.5')")
		}
	}

	// If there are format errors, stop here
	if len(errors) > 0 {
		s.renderSettingsWithErrors(w, r, errors)
		return
	}

	// Validate and fallback models against available models
	availableModels := s.GetAvailableModelIDs()
	if len(availableModels) > 0 {
		validationResult := cfg.LLM.ValidateAndFallbackModels(availableModels)

		// Add warnings for replaced models instead of errors
		if validationResult.HasReplacements {
			for modeName, replacement := range validationResult.ReplacedModels {
				if replacement.OldModel == "(empty)" {
					warnings = append(warnings, fmt.Sprintf("%s: No model was selected, defaulted to '%s'", modeName, replacement.NewModel))
				} else {
					warnings = append(warnings, fmt.Sprintf("%s: Model '%s' is not available, fell back to '%s'", modeName, replacement.OldModel, replacement.NewModel))
				}
			}
		}
	}

	// Parse routing thresholds
	codeSizeThreshold, err := strconv.Atoi(r.FormValue("routing_code_size_threshold"))
	if err != nil || codeSizeThreshold < 1 {
		errors = append(errors, "Code Size Threshold must be a positive integer")
	} else {
		cfg.LLM.RoutingRules.ComplexityThresholds.CodeSizeThreshold = codeSizeThreshold //nolint:staticcheck // deprecated but kept for backward compatibility
	}

	highComplexityThreshold, err := strconv.Atoi(r.FormValue("routing_high_complexity_threshold"))
	if err != nil || highComplexityThreshold < 1 {
		errors = append(errors, "High Complexity Threshold must be a positive integer")
	} else {
		cfg.LLM.RoutingRules.ComplexityThresholds.HighComplexityThreshold = highComplexityThreshold //nolint:staticcheck // deprecated but kept for backward compatibility
	}

	fileCountThreshold, err := strconv.Atoi(r.FormValue("routing_file_count_threshold"))
	if err != nil || fileCountThreshold < 1 {
		errors = append(errors, "File Count Threshold must be a positive integer")
	} else {
		cfg.LLM.RoutingRules.ComplexityThresholds.FileCountThreshold = fileCountThreshold //nolint:staticcheck // deprecated but kept for backward compatibility
	}

	// Parse forced strong stages
	forceStrongStagesStr := r.FormValue("routing_force_strong_stages")
	if forceStrongStagesStr != "" {
		// Split by comma and trim whitespace
		stages := strings.Split(forceStrongStagesStr, ",")
		cfg.LLM.RoutingRules.ForceStrongForStages = make([]string, 0, len(stages)) //nolint:staticcheck // deprecated but kept for backward compatibility
		for _, stage := range stages {
			stage = strings.TrimSpace(stage)
			if stage != "" {
				cfg.LLM.RoutingRules.ForceStrongForStages = append(cfg.LLM.RoutingRules.ForceStrongForStages, stage) //nolint:staticcheck // deprecated but kept for backward compatibility
			}
		}
	} else {
		cfg.LLM.RoutingRules.ForceStrongForStages = []string{} //nolint:staticcheck // deprecated but kept for backward compatibility
	}

	// Parse yolo_mode checkbox (checkbox returns "on" when checked, empty when unchecked)
	cfg.YoloMode = r.FormValue("yolo_mode") == "on"

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

	// Re-render with success message and any warnings
	forceStrongStages := strings.Join(cfg.LLM.RoutingRules.ForceStrongForStages, ", ") //nolint:staticcheck // deprecated but kept for backward compatibility
	workerCount := 0
	if s.pool != nil {
		workerCount = len(s.pool())
	}
	data := settingsData{
		Active:            "settings",
		OpenCodePort:      s.webPort,
		WorkerCount:       workerCount,
		Config:            cfg.LLM,
		YoloMode:          cfg.YoloMode,
		ForceStrongStages: forceStrongStages,
		Success:           true,
		Errors:            warnings, // Show warnings as info messages
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

	workerCount := 0
	if s.pool != nil {
		workerCount = len(s.pool())
	}
	data := settingsData{
		Active:            "settings",
		OpenCodePort:      s.webPort,
		WorkerCount:       workerCount,
		Config:            cfg.LLM,
		YoloMode:          cfg.YoloMode,
		ForceStrongStages: forceStrongStages,
		Errors:            errors,
		AvailableModels:   s.modelsCache,
	}

	s.render(w, "llm-config.html", data)
}

// validateModelFormat checks if the model string is in the correct "provider/model" format
func validateModelFormat(model string) bool {
	if model == "" {
		return false
	}
	parts := strings.Split(model, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

// validateModelSelection checks if a model selection is valid against the cached models
func (s *Server) validateModelSelection(model string) bool {
	if len(s.modelsCache) == 0 {
		return true // Skip validation if cache is empty (API unavailable)
	}

	for _, m := range s.modelsCache {
		if m.ID == model {
			return true
		}
	}
	return false
}

// handleWorkerStatus returns the current worker status as JSON
func (s *Server) handleWorkerStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.orchestrator == nil {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"active":      false,
			"paused":      true,
			"step":        "",
			"elapsed":     0,
			"issue_id":    0,
			"issue_title": "",
		}); err != nil {
			log.Printf("[Dashboard] Error encoding JSON: %v", err)
		}
		return
	}

	resp := map[string]any{
		"active":      s.orchestrator.IsProcessing(),
		"paused":      s.orchestrator.IsPaused(),
		"step":        "",
		"elapsed":     0,
		"issue_id":    0,
		"issue_title": "",
	}

	if task := s.orchestrator.CurrentTask(); task != nil {
		resp["issue_id"] = task.Issue.Number
		resp["issue_title"] = task.Issue.Title
		resp["step"] = string(task.Status)
		// Calculate elapsed time since task started
		// Note: This is a simplified version - in production you'd track actual start time
		resp["elapsed"] = 0
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[Dashboard] Error encoding JSON: %v", err)
	}
}
