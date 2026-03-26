package v2

import (
	"log"
	"net/http"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

const colApprove = "Approve"

// GetBoard returns the full board state as JSON.
func (h *Handler) GetBoard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	resp := h.buildBoardResponse()

	writeJSON(w, resp)
}

func (h *Handler) buildBoardResponse() BoardResponse {
	resp := BoardResponse{
		Blocked:       []TaskCard{},
		Backlog:       []TaskCard{},
		Plan:          []TaskCard{},
		Code:          []TaskCard{},
		AIReview:      []TaskCard{},
		CheckPipeline: []TaskCard{},
		Approve:       []TaskCard{},
		Merge:         []TaskCard{},
		Done:          []TaskCard{},
		Failed:        []TaskCard{},
		Paused:        true,
	}

	// Load YOLO mode
	if h.rootDir != "" {
		if cfg, err := config.Load(h.rootDir); err == nil {
			resp.YoloMode = cfg.YoloMode
		}
	}
	if h.yoloOverride != nil && *h.yoloOverride != nil {
		resp.YoloMode = **h.yoloOverride
	}

	if h.orchestrator != nil {
		resp.Paused = h.orchestrator.IsPaused()
		resp.Processing = h.orchestrator.IsProcessing()

		if task := h.orchestrator.CurrentTask(); task != nil {
			info := &TaskInfo{
				Number: task.Issue.Number,
				Title:  task.Issue.Title,
				Status: string(task.Status),
			}
			for _, label := range task.Issue.Labels {
				switch {
				case strings.HasPrefix(label.Name, "priority:"):
					info.Priority = strings.TrimPrefix(label.Name, "priority:")
				case label.Name == "bug" || label.Name == "type:bug":
					info.Type = "bug"
				case label.Name == "feature" || label.Name == "type:feature" || label.Name == "enhancement":
					info.Type = "feature"
				case strings.HasPrefix(label.Name, "size:"):
					info.Size = strings.TrimPrefix(label.Name, "size:")
				}
			}
			resp.CurrentTicket = info
		}
	}

	// Get active milestone name
	if h.gh != nil && h.gh.GetActiveMilestone() != nil {
		resp.SprintName = h.gh.GetActiveMilestone().Title
	}

	// If no GitHub client, no store, or no active milestone, return empty board
	if h.gh == nil || h.store == nil || h.gh.GetActiveMilestone() == nil {
		return resp
	}

	milestone := h.gh.GetActiveMilestone().Title

	issues, err := h.store.GetIssuesCacheByMilestone(milestone)
	if err != nil {
		log.Printf("[API v2] Error fetching cached issues for milestone %s: %v", milestone, err)
		return resp
	}

	for _, issue := range issues {
		col := inferColumn(issue)
		card := TaskCard{
			ID:       issue.Number,
			Title:    issue.Title,
			Status:   col,
			Assignee: issue.GetAssignee(),
			Labels:   issue.GetLabelNames(),
			IsMerged: issue.PRMerged,
		}

		// Get PR URL for approve column
		if col == colApprove && h.store != nil {
			if prURL, err := h.store.GetStepResponse(issue.Number, "create-pr"); err == nil && prURL != "" {
				card.PRURL = prURL
			}
		}

		h.addCardToColumn(&resp, col, card)
	}

	// Check if sprint can be closed
	if !resp.Processing &&
		len(resp.Blocked) == 0 &&
		len(resp.Backlog) == 0 &&
		len(resp.Plan) == 0 &&
		len(resp.Code) == 0 &&
		len(resp.AIReview) == 0 &&
		len(resp.CheckPipeline) == 0 &&
		len(resp.Approve) == 0 &&
		len(resp.Merge) == 0 {
		resp.CanCloseSprint = true
	}

	return resp
}

func inferColumn(issue github.Issue) string {
	labels := issue.GetLabelNames()
	labelSet := make(map[string]bool)
	for _, l := range labels {
		labelSet[strings.ToLower(l)] = true
	}

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
		return colApprove
	}
	if labelSet["stage:check-pipeline"] {
		return "Check Pipeline"
	}
	if labelSet["stage:create-pr"] || labelSet["stage:code-review"] {
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

func (*Handler) addCardToColumn(resp *BoardResponse, col string, card TaskCard) {
	switch col {
	case "Backlog":
		resp.Backlog = append(resp.Backlog, card)
	case "Plan":
		resp.Plan = append(resp.Plan, card)
	case "Code":
		resp.Code = append(resp.Code, card)
	case "AI Review":
		resp.AIReview = append(resp.AIReview, card)
	case "Check Pipeline":
		resp.CheckPipeline = append(resp.CheckPipeline, card)
	case colApprove:
		resp.Approve = append(resp.Approve, card)
	case "Merge":
		resp.Merge = append(resp.Merge, card)
	case "Done":
		resp.Done = append(resp.Done, card)
	case "Blocked":
		resp.Blocked = append(resp.Blocked, card)
	case "Failed":
		resp.Failed = append(resp.Failed, card)
	}
}
