package v2

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/mvp"
)

func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[API v2] Error encoding response: %v", err)
	}
}

// GetTask returns task details with steps.
func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	idStr := r.PathValue("id")
	issueNum, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, ActionResponse{Success: false, Error: "invalid task id"})
		return
	}

	resp := TaskDetailResponse{
		IssueNumber: issueNum,
		Steps:       []StepInfo{},
	}

	// Get steps from store
	if h.store != nil {
		steps, err := h.store.GetSteps(issueNum)
		if err != nil {
			log.Printf("[API v2] Error getting steps for #%d: %v", issueNum, err)
		} else {
			for _, s := range steps {
				si := StepInfo{
					ID:       s.ID,
					StepName: s.StepName,
					Status:   s.Status,
					Prompt:   s.Prompt,
					Response: s.Response,
					ErrorMsg: s.ErrorMsg,
				}
				if s.StartedAt != nil {
					t := s.StartedAt.Format("2006-01-02T15:04:05Z")
					si.StartedAt = &t
				}
				if s.FinishedAt != nil {
					t := s.FinishedAt.Format("2006-01-02T15:04:05Z")
					si.FinishedAt = &t
				}
				resp.Steps = append(resp.Steps, si)
			}
		}
	}

	// Check if task is currently active
	if h.orchestrator != nil {
		if task := h.orchestrator.CurrentTask(); task != nil && task.Issue.Number == issueNum {
			resp.IsActive = true
			resp.Status = string(task.Status)
			resp.IssueTitle = task.Issue.Title
		}
	}

	// Fallback title from GitHub
	if resp.IssueTitle == "" && h.gh != nil {
		if issue, err := h.gh.GetIssue(issueNum); err == nil {
			resp.IssueTitle = issue.Title
		}
	}
	if resp.IssueTitle == "" {
		resp.IssueTitle = fmt.Sprintf("Issue #%d", issueNum)
	}

	writeJSON(w, resp)
}

// TaskAction handles POST /api/v2/tasks/{id}/actions/{action}.
func (h *Handler) TaskAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	idStr := r.PathValue("id")
	action := r.PathValue("action")

	issueNum, err := strconv.Atoi(idStr)
	if err != nil || issueNum == 0 {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, ActionResponse{Success: false, Error: "invalid task id"})
		return
	}

	if h.orchestrator == nil || h.gh == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		writeJSON(w, ActionResponse{Success: false, Error: "service not configured"})
		return
	}

	var actionErr error
	switch action {
	case "approve":
		actionErr = h.orchestrator.ChangeStage(issueNum, github.StageApprove, github.ReasonManualApprove)
	case "reject":
		actionErr = h.orchestrator.ChangeStage(issueNum, github.StageBacklog, github.ReasonManualReject)
	case "retry":
		h.handleRetryAction(issueNum)
		actionErr = h.orchestrator.ChangeStage(issueNum, github.StageBacklog, github.ReasonManualRetry)
	case "retry-fresh":
		h.handleRetryAction(issueNum)
		actionErr = h.orchestrator.ChangeStage(issueNum, github.StageBacklog, github.ReasonManualRetryFresh)
	case "merge":
		actionErr = h.orchestrator.SendDecision(issueNum, mvp.UserDecision{Action: "approve"})
		if actionErr != nil {
			// Fallback: direct merge
			actionErr = h.handleDirectMerge(issueNum)
		}
	case "decline":
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		actionErr = h.orchestrator.SendDecision(issueNum, mvp.UserDecision{Action: "decline", Reason: body.Reason})
		if actionErr != nil {
			_ = h.orchestrator.ChangeStage(issueNum, github.StageCode, github.ReasonManualDecline)
			if body.Reason != "" {
				comment := "**Declined** — sent back for fixes.\n\n" + body.Reason
				_ = h.gh.AddComment(issueNum, comment)
			}
			if h.store != nil {
				_ = h.store.DeleteSteps(issueNum)
			}
			actionErr = nil // handled via fallback
		}
	case "block":
		actionErr = h.orchestrator.ChangeStage(issueNum, github.StageBlocked, github.ReasonManualBlock)
	case "unblock":
		actionErr = h.orchestrator.ChangeStage(issueNum, github.StageBacklog, github.ReasonManualUnblock)
	case "process":
		actionErr = h.orchestrator.QueueManualProcess(issueNum)
	default:
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, ActionResponse{Success: false, Error: "unknown action: " + action})
		return
	}

	if actionErr != nil {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, ActionResponse{Success: false, Error: actionErr.Error()})
		return
	}

	writeJSON(w, ActionResponse{Success: true, Message: fmt.Sprintf("Action '%s' applied to #%d", action, issueNum)})
}

// handleRetryAction performs cleanup for retry/retry-fresh actions.
func (h *Handler) handleRetryAction(issueNum int) {
	// Close any open PR
	if branch, err := h.gh.FindPRBranch(issueNum); err == nil {
		if closeErr := h.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[API v2] Error closing PR for #%d: %v", issueNum, closeErr)
		}
	}

	// Delete local branch
	if h.brMgr != nil {
		branchPrefix := fmt.Sprintf("oda-%d-", issueNum)
		if localBranch := h.brMgr.FindBranchByPrefix(branchPrefix); localBranch != "" {
			if err := h.brMgr.RemoveBranch(localBranch); err != nil {
				log.Printf("[API v2] Error deleting local branch for #%d: %v", issueNum, err)
			}
		}
	}

	// Clear DB steps
	if h.store != nil {
		if err := h.store.DeleteSteps(issueNum); err != nil {
			log.Printf("[API v2] Error deleting steps for #%d: %v", issueNum, err)
		}
	}
}

// handleDirectMerge performs a direct merge when the worker is not processing.
func (h *Handler) handleDirectMerge(issueNum int) error {
	branch, err := h.gh.FindPRBranch(issueNum)
	if err != nil {
		return fmt.Errorf("finding PR for #%d: %w", issueNum, err)
	}

	_ = h.orchestrator.ChangeStage(issueNum, github.StageMerge, github.ReasonManualMerge)

	if err := h.gh.MergePR(branch); err != nil {
		if closeErr := h.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[API v2] Error closing PR for #%d: %v", issueNum, closeErr)
		}
		_ = h.orchestrator.ChangeStage(issueNum, github.StageFailed, github.ReasonManualMergeFailed)
		_ = h.gh.AddComment(issueNum, "Merge failed (likely conflict). PR closed, task moved to Failed.\n\nError: "+err.Error())
		return fmt.Errorf("merge failed for #%d: %w", issueNum, err)
	}

	_ = h.orchestrator.ChangeStage(issueNum, github.StageDone, github.ReasonManualMerge)
	h.orchestrator.CheckoutDefault()
	return nil
}
