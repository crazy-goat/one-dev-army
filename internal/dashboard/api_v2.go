package dashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	ghpkg "github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/mvp"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/version"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	defaultVersion     = "0.0.0"
	workerStatusPaused = "paused"
)

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// writeJSON writes a JSON response with proper headers.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[API v2] Error encoding JSON: %v", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// parseIssueID extracts issue number from URL path.
func parseIssueID(r *http.Request) (int, error) {
	idStr := r.PathValue("id")
	n, err := strconv.Atoi(idStr)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid issue ID: %q", idStr)
	}
	return n, nil
}

// decodeJSON decodes a JSON request body into the target struct.
func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(r.Body).Decode(v)
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

type currentTicketV2 struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
	Type     string `json:"type,omitempty"`
	Size     string `json:"size,omitempty"`
}

type cardV2 struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Worker   string   `json:"worker,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
	Labels   []string `json:"labels"`
	PRURL    string   `json:"pr_url,omitempty"`
	IsMerged bool     `json:"is_merged"`
}

type boardResponseV2 struct {
	SprintName     string              `json:"sprint_name"`
	Paused         bool                `json:"paused"`
	Processing     bool                `json:"processing"`
	CanCloseSprint bool                `json:"can_close_sprint"`
	CanPlanSprint  bool                `json:"can_plan_sprint"`
	YoloMode       bool                `json:"yolo_mode"`
	TotalTickets   int                 `json:"total_tickets"`
	WorkerCount    int                 `json:"worker_count"`
	OpenCodePort   int                 `json:"opencode_port"`
	CurrentTicket  *currentTicketV2    `json:"current_ticket,omitempty"`
	Columns        map[string][]cardV2 `json:"columns"`
}

type stepV2 struct {
	ID                int64  `json:"id"`
	IssueNumber       int    `json:"issue_number"`
	StepName          string `json:"step_name"`
	Status            string `json:"status"`
	Prompt            string `json:"prompt,omitempty"`
	Response          string `json:"response,omitempty"`
	ErrorMsg          string `json:"error_msg,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	PlanAttachmentURL string `json:"plan_attachment_url,omitempty"`
	LLMModel          string `json:"llm_model,omitempty"`
	StartedAt         string `json:"started_at,omitempty"`
	FinishedAt        string `json:"finished_at,omitempty"`
}

type issueDetailV2 struct {
	IssueNumber int      `json:"issue_number"`
	IssueTitle  string   `json:"issue_title"`
	IsActive    bool     `json:"is_active"`
	Status      string   `json:"status,omitempty"`
	Steps       []stepV2 `json:"steps"`
}

type sprintStatusV2 struct {
	Paused        bool   `json:"paused"`
	Processing    bool   `json:"processing"`
	CurrentIssue  int    `json:"current_issue,omitempty"`
	CurrentStatus string `json:"current_status,omitempty"`
}

type workerInfoV2 struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	TaskID    int     `json:"task_id,omitempty"`
	TaskTitle string  `json:"task_title,omitempty"`
	Stage     string  `json:"stage,omitempty"`
	ElapsedMs float64 `json:"elapsed_ms,omitempty"`
}

type settingsResponseV2 struct {
	Config          config.LLMConfig         `json:"config"`
	YoloMode        bool                     `json:"yolo_mode"`
	SprintAutoStart bool                     `json:"sprint_auto_start"`
	AvailableModels []opencode.ProviderModel `json:"available_models"`
}

type wizardSessionResponseV2 struct {
	ID                 string         `json:"id"`
	Type               string         `json:"type"`
	CurrentStep        string         `json:"current_step"`
	IdeaText           string         `json:"idea_text,omitempty"`
	RefinedDescription string         `json:"refined_description,omitempty"`
	TechnicalPlanning  string         `json:"technical_planning,omitempty"`
	GeneratedTitle     string         `json:"generated_title,omitempty"`
	CustomTitle        string         `json:"custom_title,omitempty"`
	UseCustomTitle     bool           `json:"use_custom_title"`
	Priority           string         `json:"priority,omitempty"`
	Complexity         string         `json:"complexity,omitempty"`
	CreatedIssues      []CreatedIssue `json:"created_issues,omitempty"`
	AddToSprint        bool           `json:"add_to_sprint"`
	Language           string         `json:"language,omitempty"`
}

type sprintClosePreviewV2 struct {
	TagName        string          `json:"tag_name"`
	CurrentVersion string          `json:"current_version"`
	NewVersion     string          `json:"new_version"`
	BumpType       string          `json:"bump_type"`
	ReleaseTitle   string          `json:"release_title"`
	ReleaseBody    string          `json:"release_body"`
	LLMGenerated   bool            `json:"llm_generated"`
	ClosedIssues   []closedIssueV2 `json:"closed_issues"`
}

type closedIssueV2 struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

type sprintCloseResultV2 struct {
	Success        bool   `json:"success"`
	TagName        string `json:"tag_name"`
	ReleaseTitle   string `json:"release_title"`
	ReleaseURL     string `json:"release_url"`
	MilestoneTitle string `json:"milestone_title"`
	NewSprintTitle string `json:"new_sprint_title,omitempty"`
	Warning        string `json:"warning,omitempty"`
}

// ---------------------------------------------------------------------------
// Board & Issues (Tasks 3-4)
// ---------------------------------------------------------------------------

// handleBoardV2 returns the full board data as JSON.
// GET /api/v2/board
func (s *Server) handleBoardV2(w http.ResponseWriter, r *http.Request) {
	data := s.buildBoardData(r)

	resp := boardResponseV2{
		SprintName:     data.SprintName,
		Paused:         data.Paused,
		Processing:     data.Processing,
		CanCloseSprint: data.CanCloseSprint,
		CanPlanSprint:  data.CanPlanSprint,
		YoloMode:       data.YoloMode,
		TotalTickets:   data.TotalTickets,
		WorkerCount:    data.WorkerCount,
		OpenCodePort:   s.webPort,
		Columns:        make(map[string][]cardV2),
	}

	if data.CurrentTicket != nil {
		resp.CurrentTicket = &currentTicketV2{
			Number:   data.CurrentTicket.Number,
			Title:    data.CurrentTicket.Title,
			Status:   data.CurrentTicket.Status,
			Priority: data.CurrentTicket.Priority,
			Type:     data.CurrentTicket.Type,
			Size:     data.CurrentTicket.Size,
		}
	}

	// Convert each column's task cards to JSON-friendly structs.
	columns := map[string][]taskCard{
		"blocked":        data.Blocked,
		"backlog":        data.Backlog,
		"plan":           data.Plan,
		"code":           data.Code,
		"ai_review":      data.AIReview,
		"check_pipeline": data.CheckPipeline,
		"approve":        data.Approve,
		"merge":          data.Merge,
		"done":           data.Done,
		"failed":         data.Failed,
	}

	for colName, cards := range columns {
		v2Cards := make([]cardV2, 0, len(cards))
		for _, c := range cards {
			v2Cards = append(v2Cards, cardV2(c))
		}
		resp.Columns[colName] = v2Cards
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleIssueDetailV2 returns issue detail with steps.
// GET /api/v2/issues/{id}
func (s *Server) handleIssueDetailV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	detail := s.buildIssueDetail(issueNum)
	writeJSON(w, http.StatusOK, detail)
}

// handleIssueStepsV2 returns pipeline steps only.
// GET /api/v2/issues/{id}/steps
func (s *Server) handleIssueStepsV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	steps := s.getStepsV2(issueNum)
	writeJSON(w, http.StatusOK, map[string]any{
		"issue_number": issueNum,
		"steps":        steps,
	})
}

// buildIssueDetail constructs the issue detail response, reusing existing logic.
func (s *Server) buildIssueDetail(issueNum int) issueDetailV2 {
	detail := issueDetailV2{
		IssueNumber: issueNum,
	}

	// Get steps from DB
	detail.Steps = s.getStepsV2(issueNum)

	// Check if issue is currently active
	if s.orchestrator != nil {
		if task := s.orchestrator.CurrentTask(); task != nil && task.Issue.Number == issueNum {
			detail.IsActive = true
			detail.Status = string(task.Status)
			detail.IssueTitle = task.Issue.Title
		}
	}

	// Fallback title from GitHub
	if detail.IssueTitle == "" && s.gh != nil {
		if issue, err := s.gh.GetIssue(issueNum); err == nil {
			detail.IssueTitle = issue.Title
		}
	}
	if detail.IssueTitle == "" {
		detail.IssueTitle = fmt.Sprintf("Issue #%d", issueNum)
	}

	return detail
}

// getStepsV2 fetches steps from the DB and converts them to the v2 format.
func (s *Server) getStepsV2(issueNum int) []stepV2 {
	if s.store == nil {
		return []stepV2{}
	}

	steps, err := s.store.GetSteps(issueNum)
	if err != nil {
		log.Printf("[API v2] Error getting steps for #%d: %v", issueNum, err)
		return []stepV2{}
	}

	return convertSteps(steps)
}

func convertSteps(steps []db.TaskStep) []stepV2 {
	result := make([]stepV2, 0, len(steps))
	for _, st := range steps {
		v2 := stepV2{
			ID:                st.ID,
			IssueNumber:       st.IssueNumber,
			StepName:          st.StepName,
			Status:            st.Status,
			Prompt:            st.Prompt,
			Response:          st.Response,
			ErrorMsg:          st.ErrorMsg,
			SessionID:         st.SessionID,
			PlanAttachmentURL: st.PlanAttachmentURL,
			LLMModel:          st.LLMModel,
		}
		if st.StartedAt != nil {
			v2.StartedAt = st.StartedAt.Format("2006-01-02T15:04:05Z")
		}
		if st.FinishedAt != nil {
			v2.FinishedAt = st.FinishedAt.Format("2006-01-02T15:04:05Z")
		}
		result = append(result, v2)
	}
	return result
}

// ---------------------------------------------------------------------------
// Sprint (Tasks 4, 6)
// ---------------------------------------------------------------------------

// handleSprintStatusV2 returns sprint status as JSON.
// GET /api/v2/sprint/status
func (s *Server) handleSprintStatusV2(w http.ResponseWriter, _ *http.Request) {
	resp := sprintStatusV2{Paused: true}

	if s.orchestrator != nil {
		resp.Paused = s.orchestrator.IsPaused()
		resp.Processing = s.orchestrator.IsProcessing()
		if task := s.orchestrator.CurrentTask(); task != nil {
			resp.CurrentIssue = task.Issue.Number
			resp.CurrentStatus = string(task.Status)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleSprintStartV2 starts the orchestrator.
// POST /api/v2/sprint/start
func (s *Server) handleSprintStartV2(w http.ResponseWriter, _ *http.Request) {
	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not configured")
		return
	}
	s.orchestrator.Start()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"paused":  s.orchestrator.IsPaused(),
	})
}

// handleSprintPauseV2 pauses the orchestrator.
// POST /api/v2/sprint/pause
func (s *Server) handleSprintPauseV2(w http.ResponseWriter, _ *http.Request) {
	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not configured")
		return
	}
	s.orchestrator.Pause()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"paused":  s.orchestrator.IsPaused(),
	})
}

// handlePlanSprintV2 plans the sprint by assigning backlog tickets.
// POST /api/v2/sprint/plan
func (*Server) handlePlanSprintV2(w http.ResponseWriter, _ *http.Request) {
	// Currently a no-op placeholder - the old handler just redirects.
	// In the future, this could trigger automatic sprint planning.
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Sprint planning is not yet implemented",
	})
}

// handleVersionV2 returns the current version from the default branch.
// GET /api/v2/version
func (s *Server) handleVersionV2(w http.ResponseWriter, _ *http.Request) {
	currentVersion := defaultVersion
	if s.gh != nil {
		if version, err := s.gh.GetLatestTagFromDefaultBranch(); err == nil {
			currentVersion = version
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version": currentVersion,
	})
}

// handleSprintClosePreviewV2 generates release notes preview without closing.
// POST /api/v2/sprint/close/preview
func (s *Server) handleSprintClosePreviewV2(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BumpType string `json:"bump_type"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if body.BumpType != bumpTypeMajor && body.BumpType != bumpTypeMinor && body.BumpType != bumpTypePatch {
		body.BumpType = bumpTypePatch
	}

	if s.gh == nil || s.gh.GetActiveMilestone() == nil {
		writeError(w, http.StatusBadRequest, "no active milestone")
		return
	}

	milestone := s.gh.GetActiveMilestone()

	// Get current version
	currentVersion := defaultVersion
	if latest, err := s.gh.GetLatestTag(); err == nil {
		currentVersion = latest
	}

	// Calculate new version
	newVersion := calculateNewVersion(currentVersion, body.BumpType)
	tagName := "v" + newVersion

	// Get closed issues for this milestone
	closedIssues, err := s.gh.GetClosedIssuesForMilestone(milestone.Number)
	if err != nil {
		log.Printf("[API v2] Warning: failed to get closed issues for milestone %s: %v", milestone.Title, err)
	}

	// Generate release notes using LLM
	var releaseTitle, releaseBody string
	var llmGenerated bool

	if len(closedIssues) > 0 && s.oc != nil {
		issueList := make([]string, len(closedIssues))
		for i, issue := range closedIssues {
			issueList[i] = fmt.Sprintf("#%d: %s", issue.Number, issue.Title)
		}

		llmSession, err := s.oc.CreateSession("Release Notes Preview")
		if err == nil {
			defer func() {
				if delErr := s.oc.DeleteSession(llmSession.ID); delErr != nil {
					log.Printf("[API v2] Warning: failed to delete LLM session %s: %v", llmSession.ID, delErr)
				}
			}()

			prompt := BuildReleaseNotesPrompt(milestone.Title, tagName, issueList)
			model := opencode.ParseModelRef(s.wizardLLM)

			ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
			defer cancel()

			var result ReleaseNotes
			if err := s.oc.SendMessageStructured(ctx, llmSession.ID, prompt, model, ReleaseNotesSchema, &result); err == nil {
				releaseTitle = result.Title
				releaseBody = result.Description
				llmGenerated = true
			}
		}
	}

	// Fallback
	if releaseTitle == "" {
		releaseTitle = fmt.Sprintf("Release %s - %s", tagName, milestone.Title)
	}
	if releaseBody == "" {
		if len(closedIssues) > 0 {
			var b strings.Builder
			fmt.Fprintf(&b, "## Closed Issues in %s\n\n", milestone.Title)
			for _, issue := range closedIssues {
				fmt.Fprintf(&b, "- #%d: %s\n", issue.Number, issue.Title)
			}
			releaseBody = b.String()
		} else {
			releaseBody = fmt.Sprintf("Release %s - %s\n\nNo issues were closed in this sprint.", tagName, milestone.Title)
		}
	}

	issues := make([]closedIssueV2, len(closedIssues))
	for i, issue := range closedIssues {
		issues[i] = closedIssueV2{Number: issue.Number, Title: issue.Title}
	}

	writeJSON(w, http.StatusOK, sprintClosePreviewV2{
		TagName:        tagName,
		CurrentVersion: currentVersion,
		NewVersion:     newVersion,
		BumpType:       body.BumpType,
		ReleaseTitle:   releaseTitle,
		ReleaseBody:    releaseBody,
		LLMGenerated:   llmGenerated,
		ClosedIssues:   issues,
	})
}

// handleSprintCloseConfirmV2 creates the tag, release, and closes the sprint.
// POST /api/v2/sprint/close/confirm
func (s *Server) handleSprintCloseConfirmV2(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BumpType     string `json:"bump_type"`
		ReleaseTitle string `json:"release_title"`
		ReleaseBody  string `json:"release_body"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if s.orchestrator != nil && s.orchestrator.IsProcessing() {
		writeError(w, http.StatusConflict, "cannot close sprint while processing tasks")
		return
	}

	if s.gh == nil || s.gh.GetActiveMilestone() == nil {
		writeError(w, http.StatusBadRequest, "no active milestone")
		return
	}

	if body.BumpType != bumpTypeMajor && body.BumpType != bumpTypeMinor && body.BumpType != bumpTypePatch {
		writeError(w, http.StatusBadRequest, "invalid bump_type: must be major, minor, or patch")
		return
	}

	milestone := s.gh.GetActiveMilestone()

	// Get current version
	currentVersion := defaultVersion
	if latest, err := s.gh.GetLatestTag(); err == nil {
		currentVersion = latest
	}

	newVersion := calculateNewVersion(currentVersion, body.BumpType)
	tagName := "v" + newVersion

	// Check if tag already exists
	exists, err := s.gh.TagExists(tagName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to check tag existence: %v", err))
		return
	}
	if exists {
		writeError(w, http.StatusConflict, fmt.Sprintf("tag %s already exists", tagName))
		return
	}

	// Create the tag on master branch
	tagMessage := fmt.Sprintf("Release %s - %s", tagName, milestone.Title)
	if err := s.gh.CreateTag(tagName, "master", tagMessage); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create tag: %v", err))
		return
	}

	log.Printf("[API v2] Created tag %s for milestone %s", tagName, milestone.Title)

	// Fallback release title/body
	releaseTitle := body.ReleaseTitle
	if releaseTitle == "" {
		releaseTitle = fmt.Sprintf("Release %s - %s", tagName, milestone.Title)
	}
	releaseBody := body.ReleaseBody
	if releaseBody == "" {
		releaseBody = "Release " + tagName
	}

	// Create GitHub release
	releaseURL := fmt.Sprintf("https://github.com/%s/releases/tag/%s", s.gh.Repo, tagName)
	if err := s.gh.CreateRelease(tagName, releaseTitle, releaseBody); err != nil {
		log.Printf("[API v2] Warning: failed to create release for tag %s: %v", tagName, err)
	} else {
		log.Printf("[API v2] Created release %s for tag %s", releaseTitle, tagName)
	}

	// Close the milestone
	if err := s.gh.CloseMilestone(milestone.Number); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to close milestone: %v", err))
		return
	}

	log.Printf("[API v2] Closed milestone: %s", milestone.Title)

	// Auto-create a new sprint
	var newSprintTitle, warning string
	newSprintTitle, err = s.gh.CreateNextSprint(milestone.Title)
	if err != nil {
		log.Printf("[API v2] Warning: failed to create next sprint: %v", err)
		warning = "Release was created successfully, but failed to create the next sprint automatically."
	} else {
		log.Printf("[API v2] Created new sprint: %s", newSprintTitle)

		newMilestone, err := s.gh.GetOldestOpenMilestone()
		switch {
		case err != nil:
			warning = "Release was created successfully, but failed to reload sprint data."
		case newMilestone == nil:
			warning = "Release was created successfully, but no active sprint is available."
		default:
			s.gh.SetActiveMilestone(newMilestone)
			if s.syncService != nil {
				s.syncService.SetActiveMilestone(newMilestone.Title)
				if syncErr := s.syncService.SyncNow(); syncErr != nil {
					log.Printf("[API v2] Warning: failed to trigger sync after sprint close: %v", syncErr)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, sprintCloseResultV2{
		Success:        true,
		TagName:        tagName,
		ReleaseTitle:   releaseTitle,
		ReleaseURL:     releaseURL,
		MilestoneTitle: milestone.Title,
		NewSprintTitle: newSprintTitle,
		Warning:        warning,
	})
}

// calculateNewVersion computes the bumped version string.
func calculateNewVersion(currentVersion, bumpType string) string {
	v, err := version.Parse(currentVersion)
	if err != nil {
		v = version.Version{Major: 0, Minor: 0, Patch: 0}
	}
	switch bumpType {
	case bumpTypeMajor:
		return v.BumpMajor().String()
	case bumpTypeMinor:
		return v.BumpMinor().String()
	default:
		return v.BumpPatch().String()
	}
}

// ---------------------------------------------------------------------------
// Ticket Actions (Task 5)
// ---------------------------------------------------------------------------

// handleApproveV2 approves an issue (moves to Approve column).
// POST /api/v2/issues/{id}/approve
func (s *Server) handleApproveV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil || s.gh == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator or GitHub client not configured")
		return
	}

	if err := s.orchestrator.ChangeStage(issueNum, ghpkg.StageApprove, ghpkg.ReasonManualApprove); err != nil {
		log.Printf("[API v2] Error setting Approve label on #%d: %v", issueNum, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to approve #%d: %v", issueNum, err))
		return
	}

	log.Printf("[API v2] Approved #%d — moved to Approve column", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleRejectV2 rejects an issue (moves to Backlog).
// POST /api/v2/issues/{id}/reject
func (s *Server) handleRejectV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil || s.gh == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator or GitHub client not configured")
		return
	}

	if err := s.orchestrator.ChangeStage(issueNum, ghpkg.StageBacklog, ghpkg.ReasonManualReject); err != nil {
		log.Printf("[API v2] Error setting Backlog stage on #%d: %v", issueNum, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to reject #%d: %v", issueNum, err))
		return
	}

	log.Printf("[API v2] Rejected #%d — moved to Backlog", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleRetryV2 retries an issue (closes PR, deletes branch, clears steps, moves to Backlog).
// POST /api/v2/issues/{id}/retry
func (s *Server) handleRetryV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil || s.gh == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator or GitHub client not configured")
		return
	}

	// Close any open PR
	if branch, findErr := s.gh.FindPRBranch(issueNum); findErr == nil {
		log.Printf("[API v2] Closing PR for #%d (branch: %s)", issueNum, branch)
		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[API v2] Error closing PR for #%d: %v", issueNum, closeErr)
		}
	}

	// Delete local branch if it exists
	if s.brMgr != nil {
		branchPrefix := fmt.Sprintf("oda-%d-", issueNum)
		if localBranch := s.brMgr.FindBranchByPrefix(branchPrefix); localBranch != "" {
			log.Printf("[API v2] Deleting local branch %q for #%d", localBranch, issueNum)
			if delErr := s.brMgr.RemoveBranch(localBranch); delErr != nil {
				log.Printf("[API v2] Error deleting local branch %q for #%d: %v", localBranch, issueNum, delErr)
			}
		}
	}

	// Clear DB steps
	if s.store != nil {
		if delErr := s.store.DeleteSteps(issueNum); delErr != nil {
			log.Printf("[API v2] Error deleting steps for #%d: %v", issueNum, delErr)
		}
	}

	if err := s.orchestrator.ChangeStage(issueNum, ghpkg.StageBacklog, ghpkg.ReasonManualRetry); err != nil {
		log.Printf("[API v2] Error setting Backlog stage on #%d: %v", issueNum, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retry #%d: %v", issueNum, err))
		return
	}

	log.Printf("[API v2] Retry #%d — PR closed, local branch deleted, steps cleared, moved to Backlog", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleRetryFreshV2 retries an issue from scratch (same as retry but with fresh reason).
// POST /api/v2/issues/{id}/retry-fresh
func (s *Server) handleRetryFreshV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil || s.gh == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator or GitHub client not configured")
		return
	}

	// Close any open PR and delete branch
	if branch, findErr := s.gh.FindPRBranch(issueNum); findErr == nil {
		log.Printf("[API v2] Closing PR for #%d (branch: %s)", issueNum, branch)
		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[API v2] Error closing PR for #%d: %v", issueNum, closeErr)
		}
	}

	// Delete local branch if it exists
	if s.brMgr != nil {
		branchPrefix := fmt.Sprintf("oda-%d-", issueNum)
		if localBranch := s.brMgr.FindBranchByPrefix(branchPrefix); localBranch != "" {
			log.Printf("[API v2] Deleting local branch %q for #%d", localBranch, issueNum)
			if delErr := s.brMgr.RemoveBranch(localBranch); delErr != nil {
				log.Printf("[API v2] Error deleting local branch %q for #%d: %v", localBranch, issueNum, delErr)
			}
		}
	}

	// Clear DB steps BEFORE changing stage
	if s.store != nil {
		if delErr := s.store.DeleteSteps(issueNum); delErr != nil {
			log.Printf("[API v2] Error deleting steps for #%d: %v", issueNum, delErr)
		}
	}

	if err := s.orchestrator.ChangeStage(issueNum, ghpkg.StageBacklog, ghpkg.ReasonManualRetryFresh); err != nil {
		log.Printf("[API v2] Error setting Backlog stage on #%d: %v", issueNum, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retry-fresh #%d: %v", issueNum, err))
		return
	}

	log.Printf("[API v2] Retry fresh #%d — PR closed, local branch deleted, steps cleared, moved to Backlog", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleApproveMergeV2 approves and merges an issue.
// POST /api/v2/issues/{id}/approve-merge
func (s *Server) handleApproveMergeV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not configured")
		return
	}

	s.recordStep(issueNum, "approved", "Manual approval granted")

	err = s.orchestrator.SendDecision(issueNum, mvp.UserDecision{Action: "approve"})
	if err != nil {
		log.Printf("[API v2] Error sending approve decision for #%d: %v — falling back to direct merge", issueNum, err)
		s.handleDirectMergeV2(w, issueNum)
		return
	}

	log.Printf("[API v2] ✓ Sent approve decision for #%d", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleDirectMergeV2 is a fallback for when the worker is not processing the issue.
func (s *Server) handleDirectMergeV2(w http.ResponseWriter, issueNum int) {
	if s.gh == nil {
		writeError(w, http.StatusServiceUnavailable, "GitHub client not configured")
		return
	}

	branch, err := s.gh.FindPRBranch(issueNum)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no PR found for #%d: %v", issueNum, err))
		return
	}

	_ = s.orchestrator.ChangeStage(issueNum, ghpkg.StageMerge, ghpkg.ReasonManualMerge)

	log.Printf("[API v2] Direct merging PR for #%d (branch: %s)", issueNum, branch)
	if err := s.gh.MergePR(branch); err != nil {
		log.Printf("[API v2] ✗ Direct merge failed for #%d: %v", issueNum, err)
		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[API v2] Error closing PR for #%d: %v", issueNum, closeErr)
		}
		_ = s.orchestrator.ChangeStage(issueNum, ghpkg.StageFailed, ghpkg.ReasonManualMergeFailed)
		_ = s.gh.AddComment(issueNum, "Merge failed (likely conflict). PR closed, task moved to Failed.\n\nError: "+err.Error())
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("merge failed for #%d: %v", issueNum, err))
		return
	}

	_ = s.orchestrator.ChangeStage(issueNum, ghpkg.StageDone, ghpkg.ReasonManualMerge)
	s.orchestrator.CheckoutDefault()

	log.Printf("[API v2] ✓ Direct merged #%d (fallback)", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleDeclineV2 declines an issue and sends it back for fixes.
// POST /api/v2/issues/{id}/decline
func (s *Server) handleDeclineV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not configured")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	s.recordStep(issueNum, "declined", body.Reason)

	err = s.orchestrator.SendDecision(issueNum, mvp.UserDecision{Action: "decline", Reason: body.Reason})
	if err != nil {
		log.Printf("[API v2] Error sending decline decision for #%d: %v — falling back to direct stage change", issueNum, err)
		_ = s.orchestrator.ChangeStage(issueNum, ghpkg.StageCode, ghpkg.ReasonManualDecline)
		if body.Reason != "" {
			comment := "**Declined** — sent back for fixes.\n\n" + body.Reason
			_ = s.gh.AddComment(issueNum, comment)
		}
		if s.store != nil {
			_ = s.store.DeleteSteps(issueNum)
		}
	} else {
		log.Printf("[API v2] ✓ Sent decline decision for #%d", issueNum)
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleBlockV2 blocks an issue.
// POST /api/v2/issues/{id}/block
func (s *Server) handleBlockV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil || s.gh == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator or GitHub client not configured")
		return
	}

	if err := s.orchestrator.ChangeStage(issueNum, ghpkg.StageBlocked, ghpkg.ReasonManualBlock); err != nil {
		log.Printf("[API v2] Error setting Blocked stage on #%d: %v", issueNum, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to block #%d: %v", issueNum, err))
		return
	}

	log.Printf("[API v2] Blocked #%d", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleUnblockV2 unblocks an issue (moves to Backlog).
// POST /api/v2/issues/{id}/unblock
func (s *Server) handleUnblockV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil || s.gh == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator or GitHub client not configured")
		return
	}

	if err := s.orchestrator.ChangeStage(issueNum, ghpkg.StageBacklog, ghpkg.ReasonManualUnblock); err != nil {
		log.Printf("[API v2] Error setting Backlog stage on #%d: %v", issueNum, err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to unblock #%d: %v", issueNum, err))
		return
	}

	log.Printf("[API v2] Unblocked #%d — moved to Backlog", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleProcessV2 queues an issue for manual processing.
// POST /api/v2/issues/{id}/process
func (s *Server) handleProcessV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not configured")
		return
	}

	if err := s.orchestrator.QueueManualProcess(issueNum); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	log.Printf("[API v2] Manual process queued for #%d", issueNum)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Ticket #%d queued for processing", issueNum),
	})
}

// ---------------------------------------------------------------------------
// Workers (Tasks 4, 6)
// ---------------------------------------------------------------------------

// handleWorkersV2 returns worker status as JSON.
// GET /api/v2/workers
func (s *Server) handleWorkersV2(w http.ResponseWriter, _ *http.Request) {
	workers := make([]workerInfoV2, 0)

	if s.pool != nil {
		for _, wi := range s.pool() {
			workers = append(workers, workerInfoV2{
				ID:        wi.ID,
				Status:    string(wi.Status),
				TaskID:    wi.TaskID,
				TaskTitle: wi.TaskTitle,
				Stage:     wi.Stage,
				ElapsedMs: float64(wi.Elapsed.Milliseconds()),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"workers": workers,
		"paused":  s.orchestrator == nil || s.orchestrator.IsPaused(),
		"active":  s.orchestrator != nil && s.orchestrator.IsProcessing(),
	})
}

// handleWorkerToggleV2 pauses/resumes the orchestrator.
// POST /api/v2/workers/toggle
func (s *Server) handleWorkerToggleV2(w http.ResponseWriter, _ *http.Request) {
	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not configured")
		return
	}

	if s.orchestrator.IsPaused() {
		s.orchestrator.Start()
		log.Println("[API v2] Worker started via toggle")
	} else {
		s.orchestrator.Pause()
		log.Println("[API v2] Worker paused via toggle")
	}

	newPaused := s.orchestrator.IsPaused()
	isProcessing := s.orchestrator.IsProcessing()

	if s.hub != nil {
		status := workerStatusPaused
		if !newPaused {
			status = "active"
		}
		s.hub.BroadcastWorkerUpdate("worker-1", status, 0, "", "", 0)
	}

	statusMsg := "started"
	if newPaused {
		statusMsg = workerStatusPaused
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"paused":  newPaused,
		"active":  isProcessing,
		"message": "Worker " + statusMsg,
	})
}

// ---------------------------------------------------------------------------
// Settings (Tasks 4, 6)
// ---------------------------------------------------------------------------

// handleSettingsV2 returns current LLM config + available models.
// GET /api/v2/settings
func (s *Server) handleSettingsV2(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.Load(s.rootDir)
	if err != nil {
		log.Printf("[API v2] Error loading config: %v", err)
		defaultCfg := config.DefaultLLMConfig()
		cfg = &config.Config{LLM: defaultCfg}
	}

	writeJSON(w, http.StatusOK, settingsResponseV2{
		Config:          cfg.LLM,
		YoloMode:        cfg.YoloMode,
		SprintAutoStart: cfg.Sprint.AutoStart,
		AvailableModels: s.modelsCache,
	})
}

// handleSaveSettingsV2 saves LLM config from JSON body.
// PUT /api/v2/settings
func (s *Server) handleSaveSettingsV2(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Setup         string `json:"setup_model"`
		Planning      string `json:"planning_model"`
		Orchestration string `json:"orchestration_model"`
		Code          string `json:"code_model"`
		CodeHeavy     string `json:"code_heavy_model"`
		YoloMode      bool   `json:"yolo_mode"`
		AutoStart     bool   `json:"sprint_auto_start"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Load existing config
	cfg, err := config.Load(s.rootDir)
	if err != nil {
		log.Printf("[API v2] Error loading config: %v", err)
		cfg = &config.Config{LLM: config.DefaultLLMConfig()}
	}

	// Validate required fields
	var validationErrors []string
	modes := []struct {
		name  string
		model string
	}{
		{"Setup", body.Setup},
		{"Planning", body.Planning},
		{"Orchestration", body.Orchestration},
		{"Code", body.Code},
		{"Code Heavy", body.CodeHeavy},
	}

	for _, mode := range modes {
		if mode.model == "" {
			validationErrors = append(validationErrors, mode.name+": Model is required")
		}
	}

	if len(validationErrors) > 0 {
		writeError(w, http.StatusBadRequest, strings.Join(validationErrors, "; "))
		return
	}

	// Apply changes
	cfg.LLM.Setup.Model = body.Setup
	cfg.LLM.Planning.Model = body.Planning
	cfg.LLM.Orchestration.Model = body.Orchestration
	cfg.LLM.Code.Model = body.Code
	cfg.LLM.CodeHeavy.Model = body.CodeHeavy
	cfg.YoloMode = body.YoloMode
	cfg.Sprint.AutoStart = body.AutoStart

	// Save the config
	if err := config.SaveConfig(s.rootDir, cfg); err != nil {
		log.Printf("[API v2] Error saving config: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save configuration: %v", err))
		return
	}

	log.Printf("[API v2] LLM configuration saved successfully")

	// Propagate immediately
	if s.configPropagator != nil {
		s.configPropagator.Propagate(cfg)
		log.Printf("[API v2] Configuration propagated immediately to all workers")
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"config":  cfg.LLM,
	})
}

// handleYoloToggleV2 toggles YOLO mode.
// POST /api/v2/settings/yolo
func (s *Server) handleYoloToggleV2(w http.ResponseWriter, _ *http.Request) {
	if s.rootDir == "" {
		writeError(w, http.StatusInternalServerError, "root directory not configured")
		return
	}

	cfg, err := config.Load(s.rootDir)
	if err != nil {
		log.Printf("[API v2] Error loading config for YOLO toggle: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load configuration")
		return
	}

	// Determine current state from runtime override or config
	currentYolo := cfg.YoloMode
	if s.yoloOverride != nil {
		currentYolo = *s.yoloOverride
	}

	// Toggle
	newYolo := !currentYolo
	s.yoloOverride = &newYolo

	// Propagate in-memory to workers (no file save)
	cfg.YoloMode = newYolo
	if s.configPropagator != nil {
		s.configPropagator.Propagate(cfg)
	}

	log.Printf("[API v2] YOLO mode toggled to: %v (runtime only)", newYolo)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"yolo_mode": newYolo,
	})
}

// ---------------------------------------------------------------------------
// Sync (Task 6)
// ---------------------------------------------------------------------------

// handleSyncV2 triggers a manual sync.
// POST /api/v2/sync
func (s *Server) handleSyncV2(w http.ResponseWriter, _ *http.Request) {
	if s.syncService == nil {
		writeError(w, http.StatusServiceUnavailable, "sync service not configured")
		return
	}

	if err := s.syncService.SyncNow(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.hub != nil {
		s.hub.BroadcastSyncComplete(0)
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ---------------------------------------------------------------------------
// Rate Limit (Task 4)
// ---------------------------------------------------------------------------

// handleRateLimitV2 returns GitHub API rate limit as JSON.
// GET /api/v2/rate-limit
func (s *Server) handleRateLimitV2(w http.ResponseWriter, _ *http.Request) {
	if s.rateLimitService == nil {
		writeError(w, http.StatusServiceUnavailable, "rate limit service not configured")
		return
	}

	summary := s.rateLimitService.GetSummary()
	if summary == nil {
		writeError(w, http.StatusServiceUnavailable, "rate limit data not available")
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// ---------------------------------------------------------------------------
// Wizard (Task 7)
// ---------------------------------------------------------------------------

// handleWizardCreateSessionV2 creates a new wizard session.
// POST /api/v2/wizard/sessions
func (s *Server) handleWizardCreateSessionV2(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type string `json:"type"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	session, err := s.wizardStore.Create(body.Type)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, wizardSessionToResponse(session))
}

// handleWizardGetSessionV2 returns session state.
// GET /api/v2/wizard/sessions/{id}
func (s *Server) handleWizardGetSessionV2(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, wizardSessionToResponse(session))
}

// handleWizardDeleteSessionV2 cancels/deletes a session.
// DELETE /api/v2/wizard/sessions/{id}
func (s *Server) handleWizardDeleteSessionV2(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	s.wizardStore.Delete(sessionID)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleWizardRefineV2 sends the idea to LLM for refinement.
// POST /api/v2/wizard/sessions/{id}/refine
func (s *Server) handleWizardRefineV2(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var body struct {
		Idea     string `json:"idea"`
		Language string `json:"language"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if body.Idea == "" {
		writeError(w, http.StatusBadRequest, "idea is required")
		return
	}

	const maxIdeaLength = 10000
	if len(body.Idea) > maxIdeaLength {
		writeError(w, http.StatusBadRequest, "input exceeds maximum length of 10000 characters")
		return
	}

	session.SetIdeaText(body.Idea)
	if body.Language != "" {
		session.SetLanguage(body.Language)
	}
	session.SetSkipBreakdown(true)
	session.SetStep(WizardStepRefine)
	session.AddLog("user", body.Idea)

	// If no opencode client, return mock response for testing
	if s.oc == nil {
		mockTitle := "[Feature] Add user authentication system"
		if session.Type == WizardTypeBug {
			mockTitle = "[Bug] Fix authentication issue"
		}
		mockPlanning := "## Description\n\nAdd user authentication to the system.\n\n## Tasks\n\n1. Create auth service\n2. Add user storage\n3. Add login endpoint\n4. Write tests"
		session.SetTechnicalPlanning(mockPlanning)
		session.SetGeneratedTitle(mockTitle)
		session.SetPriority("medium")
		session.SetComplexity("M")
		session.AddLog("assistant", "Generated title: "+mockTitle+"\n\n"+mockPlanning)

		writeJSON(w, http.StatusOK, wizardSessionToResponse(session))
		return
	}

	// Create LLM session
	llmSession, err := s.oc.CreateSession("Wizard Issue Generation")
	if err != nil {
		log.Printf("[API v2] Error creating LLM session: %v", err)
		session.AddLog("system", "Error: Failed to create LLM session - "+err.Error())
		writeError(w, http.StatusInternalServerError, "failed to connect to AI service")
		return
	}
	defer func() {
		if delErr := s.oc.DeleteSession(llmSession.ID); delErr != nil {
			log.Printf("[API v2] Error deleting LLM session %s: %v", llmSession.ID, delErr)
		}
	}()

	codebaseContext := GetCodebaseContext()
	prompt := BuildIssueGenerationPrompt(session.Type, body.Idea, codebaseContext, session.Language)
	session.AddLog("system", "Sending issue generation request to LLM")

	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(s.wizardLLM)
	var result GeneratedIssue
	err = s.oc.SendMessageStructured(ctx, llmSession.ID, prompt, model, GeneratedIssueSchema, &result)
	if err != nil {
		log.Printf("[API v2] Error from LLM: %v", err)
		session.AddLog("system", "LLM error: "+err.Error())
		errorMsg := "Failed to generate issue. "
		if ctx.Err() == context.DeadlineExceeded {
			errorMsg += "The AI service timed out. Please try again with a shorter description."
		} else {
			errorMsg += "Please check your connection and try again."
		}
		writeError(w, http.StatusInternalServerError, errorMsg)
		return
	}

	if result.Description == "" {
		session.AddLog("system", "Error: LLM returned empty description")
		writeError(w, http.StatusInternalServerError, "the AI returned an empty response")
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

	writeJSON(w, http.StatusOK, wizardSessionToResponse(session))
}

// handleWizardCreateIssueV2 creates a GitHub issue from the wizard session.
// POST /api/v2/wizard/sessions/{id}/create
func (s *Server) handleWizardCreateIssueV2(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var body struct {
		Title       string `json:"title"`
		AddToSprint bool   `json:"add_to_sprint"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	session.SetAddToSprint(body.AddToSprint)
	if body.Title != "" {
		session.SetCustomTitle(body.Title)
		session.SetUseCustomTitle(true)
	}

	session.SetStep(WizardStepCreate)
	session.AddLog("system", "Creating single issue from technical planning")

	// If no GitHub client, return mock confirmation
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
		s.wizardStore.Delete(session.ID)

		writeJSON(w, http.StatusCreated, map[string]any{
			"success": true,
			"issue":   mockIssue,
		})
		return
	}

	// Build labels
	labels := []string{"wizard"}
	switch session.Type {
	case WizardTypeFeature:
		labels = append(labels, "enhancement")
	case WizardTypeBug:
		labels = append(labels, "bug")
	}

	gi := GeneratedIssue{Priority: session.Priority, Complexity: session.Complexity}
	if label := gi.PriorityLabel(); label != "" {
		labels = append(labels, label)
	}
	if label := gi.ComplexityLabel(); label != "" {
		labels = append(labels, label)
	}

	title := session.GetFinalTitle()
	if title == "" {
		if session.Type == WizardTypeBug {
			title = defaultBugTitle
		} else {
			title = defaultFeatureTitle
		}
	}
	if len(title) > 80 {
		title = title[:77] + "..."
	}

	issueBody := session.TechnicalPlanning
	issueNum, err := s.gh.CreateIssue(title, issueBody, labels)
	if err != nil {
		log.Printf("[API v2] Error creating single issue: %v", err)
		session.AddLog("system", fmt.Sprintf("Error creating single issue: %v", err))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create issue: %v", err))
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
	if body.AddToSprint && sprintName != "" {
		if err := s.gh.SetMilestone(issueNum, sprintName); err != nil {
			log.Printf("[API v2] Error assigning #%d to sprint %s: %v", issueNum, sprintName, err)
			session.AddLog("system", fmt.Sprintf("Warning: could not assign to sprint: %v", err))
		} else {
			session.AddLog("system", fmt.Sprintf("Assigned #%d to %s", issueNum, sprintName))
		}
	}

	// Trigger immediate sync
	if s.syncService != nil {
		go func() {
			if syncErr := s.syncService.SyncNow(); syncErr != nil {
				log.Printf("[API v2] Sync error after issue creation: %v", syncErr)
			}
		}()
	}

	// Clean up session
	s.wizardStore.Delete(session.ID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"issue":   issue,
	})
}

// handleWizardLogsV2 returns LLM interaction logs for a session.
// GET /api/v2/wizard/sessions/{id}/logs
func (s *Server) handleWizardLogsV2(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	session, ok := s.wizardStore.Get(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	logs := session.GetLogs()
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID,
		"logs":       logs,
	})
}

// wizardSessionToResponse converts a WizardSession to the API response struct.
func wizardSessionToResponse(session *WizardSession) wizardSessionResponseV2 {
	session.mu.RLock()
	defer session.mu.RUnlock()

	return wizardSessionResponseV2{
		ID:                 session.ID,
		Type:               string(session.Type),
		CurrentStep:        string(session.CurrentStep),
		IdeaText:           session.IdeaText,
		RefinedDescription: session.RefinedDescription,
		TechnicalPlanning:  session.TechnicalPlanning,
		GeneratedTitle:     session.GeneratedTitle,
		CustomTitle:        session.CustomTitle,
		UseCustomTitle:     session.UseCustomTitle,
		Priority:           session.Priority,
		Complexity:         session.Complexity,
		CreatedIssues:      session.CreatedIssues,
		AddToSprint:        session.AddToSprint,
		Language:           session.Language,
	}
}

// ---------------------------------------------------------------------------
// SSE Streaming (Task 8)
// ---------------------------------------------------------------------------

// handleTaskStreamV2 proxies live LLM output as SSE.
// GET /api/v2/issues/{id}/stream
func (s *Server) handleTaskStreamV2(w http.ResponseWriter, r *http.Request) {
	// The existing handleTaskStream uses r.PathValue("id") — same as our route.
	s.handleTaskStream(w, r)
}

// handleLogStreamV2 tails log files as SSE.
// GET /api/v2/issues/{id}/logs/stream
//
// The existing handleLogStream reads r.PathValue("issue"), but our v2 route
// uses {id}. We set the path value manually so the existing handler can read it.
func (s *Server) handleLogStreamV2(w http.ResponseWriter, r *http.Request) {
	// The v2 route uses {id}, but the existing handler reads {issue}.
	// We delegate to a thin wrapper that extracts {id} and calls the
	// existing log-streaming logic directly.
	issueStr := r.PathValue("id")
	if _, err := strconv.Atoi(issueStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue ID")
		return
	}

	// Build a new request with the path value set as "issue" for the existing handler.
	// Since Go 1.22 ServeMux stores path values internally, we can't easily re-map them.
	// Instead, we inline the core logic from handleLogStream, passing issueStr directly.
	s.streamLogsForIssue(w, r, issueStr)
}

// streamLogsForIssue is extracted from handleLogStream to allow reuse with
// different path parameter names. It streams log files for the given issue.
func (s *Server) streamLogsForIssue(w http.ResponseWriter, r *http.Request, issueStr string) {
	stepFilter := r.URL.Query().Get("step")
	follow := r.URL.Query().Get("follow") != "false"

	logDir := filepath.Join(s.rootDir, ".oda", "artifacts", issueStr, "logs")

	info, err := os.Stat(logDir)
	if err != nil || !info.IsDir() {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		sendSSEEvent(w, flusher, "log:error", map[string]string{
			"error": "log directory not found for issue " + issueStr,
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	entries, err := os.ReadDir(logDir)
	if err != nil {
		sendSSEEvent(w, flusher, "log:error", map[string]string{
			"error": fmt.Sprintf("failed to read log directory: %v", err),
		})
		return
	}

	type fileOffset struct {
		offset   int64
		complete bool
	}
	fileOffsets := make(map[string]fileOffset)

	var logFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		if stepFilter != "" {
			stepName := strings.TrimSuffix(name, ".log")
			if idx := strings.LastIndex(stepName, "_"); idx != -1 {
				stepName = stepName[idx+1:]
			}
			if stepName != stepFilter {
				continue
			}
		}
		logFiles = append(logFiles, name)
	}

	sort.Strings(logFiles)

	streamFile := func(filename string) {
		fp := filepath.Join(logDir, filename)
		file, openErr := os.Open(fp)
		if openErr != nil {
			sendSSEEvent(w, flusher, "log:error", map[string]string{
				"error": fmt.Sprintf("failed to open log file %s: %v", filename, openErr),
			})
			return
		}
		defer func() { _ = file.Close() }()

		offset := fileOffsets[filename].offset
		if offset > 0 {
			if _, seekErr := file.Seek(offset, 0); seekErr != nil {
				sendSSEEvent(w, flusher, "log:error", map[string]string{
					"error": fmt.Sprintf("failed to seek in log file %s: %v", filename, seekErr),
				})
				return
			}
		}

		scanner := bufio.NewScanner(file)
		stepName := strings.TrimSuffix(filename, ".log")
		if idx := strings.LastIndex(stepName, "_"); idx != -1 {
			stepName = stepName[idx+1:]
		}

		for scanner.Scan() {
			line := scanner.Text()
			timestamp := ""
			message := line
			if len(line) > 22 && line[0] == '[' && line[5] == '-' && line[20] == ']' {
				timestamp = line[1:20]
				if len(line) > 22 {
					message = line[22:]
				}
			}

			sendSSEEvent(w, flusher, "log:new", map[string]string{
				"file":      filename,
				"step":      stepName,
				"timestamp": timestamp,
				"message":   message,
			})

			if strings.Contains(line, "STEP END:") {
				fileOffsets[filename] = fileOffset{offset: offset, complete: true}
				sendSSEEvent(w, flusher, "log:complete", map[string]string{
					"file": filename,
					"step": stepName,
				})
				return
			}
		}

		currentOffset, _ := file.Seek(0, 1)
		fileOffsets[filename] = fileOffset{offset: currentOffset, complete: false}
	}

	for _, filename := range logFiles {
		streamFile(filename)
	}

	if !follow {
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			allComplete := true
			for _, filename := range logFiles {
				if !fileOffsets[filename].complete {
					allComplete = false
					streamFile(filename)
				}
			}
			if allComplete {
				return
			}
		}
	}
}
