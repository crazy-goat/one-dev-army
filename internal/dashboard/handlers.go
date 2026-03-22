package dashboard

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/worker"
)

type taskCard struct {
	ID       int
	Title    string
	Status   string
	Worker   string
	Assignee string
	Labels   []string
}

type boardData struct {
	Active     string
	SprintName string
	Backlog    []taskCard
	Progress   []taskCard
	Review     []taskCard
	Merging    []taskCard
	Done       []taskCard
	Blocked    []taskCard
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
		Review: []taskCard{
			{ID: 4, Title: "Database migrations", Status: "review"},
		},
		Merging: []taskCard{},
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
		// Use project items to determine column
		for _, col := range github.ProjectColumns {
			items := itemsByStatus[col]
			for _, item := range items {
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
	} else {
		// Infer column from issue state and labels
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
	if labelSet["blocked"] || labelSet["blocker"] {
		return "Blocked"
	}
	if labelSet["in-progress"] || labelSet["in progress"] || labelSet["wip"] || labelSet["working"] {
		return "In Progress"
	}
	if labelSet["review"] || labelSet["in-review"] || labelSet["pr-ready"] {
		return "Review"
	}
	if labelSet["merging"] || labelSet["merge-ready"] {
		return "Merging"
	}
	if labelSet["done"] || labelSet["completed"] || labelSet["finished"] {
		return "Done"
	}

	// Check state
	if issue.State == "CLOSED" {
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
	case "Review":
		data.Review = append(data.Review, card)
	case "Merging":
		data.Merging = append(data.Merging, card)
	case "Done":
		data.Done = append(data.Done, card)
	case "Blocked":
		data.Blocked = append(data.Blocked, card)
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
