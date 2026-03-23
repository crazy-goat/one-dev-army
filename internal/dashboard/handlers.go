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
	DoneFilter     string // Filter for Done column: "all", "merged", "closed"
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

	// Get filter from query parameter
	if r != nil {
		data.DoneFilter = r.URL.Query().Get("done_filter")
	}
	if data.DoneFilter == "" {
		data.DoneFilter = "all"
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

	// Apply Done column filter
	if data.DoneFilter != "all" && len(data.Done) > 0 {
		var filteredDone []taskCard
		for _, card := range data.Done {
			if data.DoneFilter == "merged" && card.IsMerged {
				filteredDone = append(filteredDone, card)
			} else if data.DoneFilter == "closed" && !card.IsMerged {
				filteredDone = append(filteredDone, card)
			}
		}
		data.Done = filteredDone
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

	// Set stage label to "Approve"
	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Approve")
	if err != nil {
		log.Printf("[Dashboard] Error setting Approve label on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Update cache
	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	// Broadcast update via WebSocket
	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
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

	// Set stage label to "Backlog" (removes all stage labels)
	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Backlog")
	if err != nil {
		log.Printf("[Dashboard] Error setting Backlog stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Update cache
	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	// Broadcast update via WebSocket
	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
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

	// Set stage label to "Code" (removes failed label and adds coding label)
	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Code")
	if err != nil {
		log.Printf("[Dashboard] Error setting Code stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Update cache
	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	// Broadcast update via WebSocket
	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
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

	// Set stage label to "Backlog" (removes all stage labels)
	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Backlog")
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

	// Update cache
	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	// Broadcast update via WebSocket
	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
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

	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Blocked")
	if err != nil {
		log.Printf("[Dashboard] Error setting Blocked stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
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

	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Backlog")
	if err != nil {
		log.Printf("[Dashboard] Error setting Backlog stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
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
	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Code")
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

	// Update cache
	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	// Broadcast update via WebSocket
	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
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
	mergingIssue, mergeStageErr := s.gh.SetStageLabel(issueNum, "Merge")
	if mergeStageErr != nil {
		log.Printf("[Dashboard] Error setting Merge stage on #%d: %v", issueNum, mergeStageErr)
	} else {
		if s.store != nil {
			milestone := s.activeSprintName()
			_ = s.store.SaveIssueCache(mergingIssue, milestone)
		}
		if s.hub != nil {
			s.hub.BroadcastIssueUpdate(mergingIssue)
		}
	}

	log.Printf("[Dashboard] Approving & merging PR for #%d (branch: %s)", issueNum, branch)
	if err := s.gh.MergePR(branch); err != nil {
		log.Printf("[Dashboard] ✗ Merge failed for #%d (likely conflict): %v", issueNum, err)
		s.recordStep(issueNum, "merge-failed", err.Error())

		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[Dashboard] Error closing PR for #%d: %v", issueNum, closeErr)
		}

		// Set stage label to "Failed" on merge failure
		updatedIssue, labelErr := s.gh.SetStageLabel(issueNum, "Failed")
		if labelErr != nil {
			log.Printf("[Dashboard] Error setting Failed stage on #%d: %v", issueNum, labelErr)
		} else {
			if s.store != nil {
				milestone := s.activeSprintName()
				if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
					log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
				}
			}
			if s.hub != nil {
				s.hub.BroadcastIssueUpdate(updatedIssue)
			}
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
	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Done")
	if err != nil {
		log.Printf("[Dashboard] Error setting Done stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Update cache
	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	// Broadcast update via WebSocket
	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
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

	// Clear active milestone
	s.gh.SetActiveMilestone(nil)

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
		mockPlanning := "## Problem Statement / Feature Description\n\nAdd user authentication to the system.\n\n## Architecture Overview\n\nKey components involved: auth service, user database, session management.\n\n## Files Requiring Changes\n\n- `internal/auth/service.go`: Add authentication logic\n- `internal/db/users.go`: Add user storage\n\n## Component Dependencies\n\n- Database for user storage\n- Session management library\n\n## Implementation Boundaries\n\n- In scope: Login/logout functionality\n- Out of scope: Password reset, OAuth\n\n## Acceptance Criteria\n\n- [ ] Users can log in with username and password\n- [ ] Sessions are maintained across requests\n- [ ] Invalid credentials are rejected"
		session.SetTechnicalPlanning(mockPlanning)
		session.AddLog("assistant", mockPlanning)

		data := struct {
			SessionID          string
			Type               string
			TechnicalPlanning  string
			IsPage             bool
			SkipBreakdown      bool
			SprintName         string
			CurrentStep        int
			ShowBreakdownStep  bool
			NeedsTypeSelection bool
		}{
			SessionID:          session.ID,
			Type:               string(session.Type),
			TechnicalPlanning:  mockPlanning,
			IsPage:             isPage,
			SkipBreakdown:      true, // Always skip breakdown in new flow
			SprintName:         s.activeSprintName(),
			CurrentStep:        2,     // Now step 2 is Technical Planning
			ShowBreakdownStep:  false, // No more breakdown step
			NeedsTypeSelection: false,
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

	// Build unified technical planning prompt with codebase context
	codebaseContext := GetCodebaseContext()
	prompt := BuildTechnicalPlanningPrompt(session.Type, inputText, codebaseContext, session.Language)
	session.AddLog("system", "Sending technical planning request to LLM (language: "+session.Language+")")

	// Send message to LLM with timeout
	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(s.wizardLLM)
	var output strings.Builder
	response, err := s.oc.SendMessageStream(ctx, llmSession.ID, prompt, model, &output)
	if err != nil {
		log.Printf("[Wizard] Error from LLM: %v", err)
		session.AddLog("system", "LLM error: "+err.Error())

		errorMsg := "Failed to generate technical planning. "
		if ctx.Err() == context.DeadlineExceeded {
			errorMsg += "The AI service timed out. Please try again with a shorter description."
		} else {
			errorMsg += "Please check your connection and try again."
		}

		s.renderError(w, errorMsg, session.ID, string(session.Type), isPage)
		return
	}

	// Extract technical planning from response
	var technicalPlanning string
	if len(response.Parts) > 0 {
		technicalPlanning = stripLLMPreamble(response.Parts[0].Text)
	}

	// Validate that we got a non-empty response
	if technicalPlanning == "" {
		log.Printf("[Wizard] LLM returned empty response for session %s", session.ID)
		session.AddLog("system", "Error: LLM returned empty response")
		s.renderError(w, "The AI returned an empty response. Please try again with a more detailed description.", session.ID, string(session.Type), isPage)
		return
	}

	session.SetTechnicalPlanning(technicalPlanning)
	session.AddLog("assistant", technicalPlanning)

	data := struct {
		SessionID          string
		Type               string
		TechnicalPlanning  string
		IsPage             bool
		SkipBreakdown      bool
		SprintName         string
		CurrentStep        int
		ShowBreakdownStep  bool
		NeedsTypeSelection bool
	}{
		SessionID:          session.ID,
		Type:               string(session.Type),
		TechnicalPlanning:  technicalPlanning,
		IsPage:             isPage,
		SkipBreakdown:      true, // Always skip breakdown in new flow
		SprintName:         s.activeSprintName(),
		CurrentStep:        2,     // Now step 2 is Technical Planning
		ShowBreakdownStep:  false, // No more breakdown step
		NeedsTypeSelection: false,
	}

	s.renderFragment(w, "wizard_refine.html", data)
}

// handleWizardGenerateTitle generates or regenerates the issue title from technical planning
func (s *Server) handleWizardGenerateTitle(w http.ResponseWriter, r *http.Request) {
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

	// Check if this is a regeneration request
	regenerate := r.FormValue("regenerate") == "1"

	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusBadRequest)
		return
	}

	// If user provided a custom title, store it
	customTitle := r.FormValue("issue_title")
	if customTitle != "" && !regenerate {
		session.SetCustomTitle(customTitle)
		session.SetUseCustomTitle(true)
	}

	session.SetStep(WizardStepTitle)

	// If we already have a generated title and not regenerating, use it
	if session.GeneratedTitle != "" && !regenerate {
		title := session.GetFinalTitle()
		s.renderTitlePage(w, session, title, isPage)
		return
	}

	// Generate title using LLM
	if s.oc == nil {
		// Mock title generation for testing
		mockTitle := generateMockTitle(session.Type, session.TechnicalPlanning)
		session.SetGeneratedTitle(mockTitle)
		session.AddLog("system", "Mock: Generated title: "+mockTitle)

		title := session.GetFinalTitle()
		s.renderTitlePage(w, session, title, isPage)
		return
	}

	// Create LLM session for title generation
	llmSession, err := s.oc.CreateSession("Wizard Title Generation")
	if err != nil {
		log.Printf("[Wizard] Error creating LLM session for title: %v", err)
		session.AddLog("system", "Error: Failed to create LLM session for title - "+err.Error())
		s.renderError(w, "Failed to connect to AI service for title generation. Please try again.", session.ID, string(session.Type), isPage)
		return
	}
	defer func() {
		if err := s.oc.DeleteSession(llmSession.ID); err != nil {
			log.Printf("[Wizard] Error deleting LLM session %s: %v", llmSession.ID, err)
		}
	}()

	// Build title generation prompt
	prompt := BuildTitleGenerationPrompt(session.Type, session.TechnicalPlanning, session.Language)
	session.AddLog("system", "Sending title generation request to LLM")

	// Send message to LLM with timeout
	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(s.wizardLLM)
	var output strings.Builder
	response, err := s.oc.SendMessageStream(ctx, llmSession.ID, prompt, model, &output)
	if err != nil {
		log.Printf("[Wizard] Error from LLM during title generation: %v", err)
		session.AddLog("system", "LLM error during title generation: "+err.Error())

		errorMsg := "Failed to generate title. "
		if ctx.Err() == context.DeadlineExceeded {
			errorMsg += "The AI service timed out. Please try again."
		} else {
			errorMsg += "Please check your connection and try again."
		}

		s.renderError(w, errorMsg, session.ID, string(session.Type), isPage)
		return
	}

	// Extract title from response
	var generatedTitle string
	if len(response.Parts) > 0 {
		generatedTitle = strings.TrimSpace(response.Parts[0].Text)
	}

	// Validate title
	if generatedTitle == "" {
		log.Printf("[Wizard] LLM returned empty title for session %s", session.ID)
		session.AddLog("system", "Error: LLM returned empty title")
		// Fallback to a default title based on type
		if session.Type == WizardTypeBug {
			generatedTitle = "[Bug] Fix issue"
		} else {
			generatedTitle = "[Feature] New feature"
		}
	}

	// Ensure title has proper prefix
	if !strings.HasPrefix(generatedTitle, "[Feature]") && !strings.HasPrefix(generatedTitle, "[Bug]") {
		if session.Type == WizardTypeBug {
			generatedTitle = "[Bug] " + generatedTitle
		} else {
			generatedTitle = "[Feature] " + generatedTitle
		}
	}

	// Truncate if too long
	if len(generatedTitle) > 80 {
		generatedTitle = generatedTitle[:77] + "..."
	}

	session.SetGeneratedTitle(generatedTitle)
	session.AddLog("assistant", "Generated title: "+generatedTitle)

	title := session.GetFinalTitle()
	s.renderTitlePage(w, session, title, isPage)
}

// renderTitlePage renders the title page with the given title
func (s *Server) renderTitlePage(w http.ResponseWriter, session *WizardSession, title string, isPage bool) {
	data := struct {
		SessionID          string
		Type               string
		Title              string
		IsPage             bool
		SprintName         string
		CurrentStep        int
		ShowBreakdownStep  bool
		NeedsTypeSelection bool
	}{
		SessionID:          session.ID,
		Type:               string(session.Type),
		Title:              title,
		IsPage:             isPage,
		SprintName:         s.activeSprintName(),
		CurrentStep:        3, // Title is step 3
		ShowBreakdownStep:  false,
		NeedsTypeSelection: false,
	}

	s.renderFragment(w, "wizard_title.html", data)
}

// generateMockTitle generates a mock title for testing
func generateMockTitle(wizardType WizardType, technicalPlanning string) string {
	// Extract first line or first sentence for context
	lines := strings.Split(technicalPlanning, "\n")
	var context string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "-") {
			context = trimmed
			break
		}
	}

	if context == "" {
		context = "New implementation"
	}

	// Truncate context if too long
	words := strings.Fields(context)
	if len(words) > 5 {
		words = words[:5]
	}
	context = strings.Join(words, " ")

	if wizardType == WizardTypeBug {
		return "[Bug] Fix " + context
	}
	return "[Feature] Add " + context
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

// handleWizardCreateSingle creates a single GitHub issue (for the unified technical planning flow)
func (s *Server) handleWizardCreateSingle(w http.ResponseWriter, r *http.Request, session *WizardSession, isPage bool) {
	// Read sprint assignment preference from form
	addToSprint := r.FormValue("add_to_sprint") == "1"
	session.SetAddToSprint(addToSprint)

	// If no GitHub client, return mock confirmation for testing
	if s.gh == nil {
		mockTitle := session.GetFinalTitle()
		if mockTitle == "" {
			mockTitle = generateMockTitle(session.Type, session.TechnicalPlanning)
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
			CurrentStep:        4, // Step 4 is Create in new 4-step flow
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

	// Get title from session (either custom or generated)
	title := session.GetFinalTitle()
	if title == "" {
		// Fallback: generate a simple title from technical planning
		title = generateMockTitle(session.Type, session.TechnicalPlanning)
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
		CurrentStep:        4, // Step 4 is Create in new 4-step flow
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

	data := s.rateLimitService.GetData()

	// Build the HTML response
	color := data.GetColorCSS()
	statusText := fmt.Sprintf("GitHub API: %d/%d", data.Remaining, data.Limit)
	resetText := data.GetResetTimeFormatted()

	// Add warning indicator if there's an error but we have cached data
	warningIcon := ""
	if data.Error != "" && !data.UpdatedAt.IsZero() {
		warningIcon = " ⚠"
	}

	html := fmt.Sprintf(
		`<div class="rate-limit-container" style="color: %s; cursor: pointer;" title="Click to refresh" hx-post="/api/rate-limit/refresh" hx-swap="outerHTML">
			<span class="rate-limit-status">%s%s</span>
			<span class="rate-limit-reset">%s</span>
		</div>`,
		color, statusText, warningIcon, resetText,
	)

	w.Write([]byte(html))
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
