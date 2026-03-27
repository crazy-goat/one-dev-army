# Sprint Plan UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement new "Plan Sprint" page in React dashboard that allows AI-powered selection of unassigned GitHub issues to add to current sprint.

**Architecture:** React SPA page at `/sprint/plan` route with async AI proposal generation, dependency tree visualization, and SSE-based progress tracking for GitHub milestone assignment.

**Tech Stack:** React, TypeScript, Go (backend), SSE (Server-Sent Events), GitHub API, GitHub Linked Issues API

**Key Decisions from Iteration:**
- Use existing `GetCurrentSprint()` (oldest open milestone)
- GitHub Linked Issues API for dependencies (with fallback to parsing)
- Fetch last tag for AI context
- Tree view with "all or nothing" branch selection
- Soft limit 20% overcommit (AI can propose 100-120% of target)
- Target = ticket count (not complexity)
- No draft/undo/refresh features

---

## Task 1: Add Sprint Planning Config

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

Add to `internal/config/config_test.go`:
```go
func TestSprintPlanningConfig(t *testing.T) {
    cfg := &Config{
        Sprint: SprintConfig{
            Planning: SprintPlanningConfig{
                DefaultTargetCount:      10,
                MaxOvercommitPercentage: 20,
            },
        },
    }
    
    assert.Equal(t, 10, cfg.Sprint.Planning.DefaultTargetCount)
    assert.Equal(t, 20, cfg.Sprint.Planning.MaxOvercommitPercentage)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestSprintPlanningConfig -v`
Expected: FAIL - SprintPlanningConfig undefined

**Step 3: Add config structures**

Add to `internal/config/config.go`:
```go
type SprintConfig struct {
    Planning SprintPlanningConfig `yaml:"planning"`
}

type SprintPlanningConfig struct {
    DefaultTargetCount      int `yaml:"default_target_count"`
    MaxOvercommitPercentage int `yaml:"max_overcommit_percentage"`
}
```

Update Config struct:
```go
type Config struct {
    // ... existing fields
    Sprint SprintConfig `yaml:"sprint"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run TestSprintPlanningConfig -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add sprint planning configuration"
```

---

## Task 2: Add GetLastTag Endpoint

**Files:**
- Create: `internal/dashboard/api_v2_sprint.go`
- Test: `internal/dashboard/api_v2_sprint_test.go`
- Modify: `internal/github/client.go` (add GetLastTag method)

**Step 1: Write the failing test**

Create `internal/dashboard/api_v2_sprint_test.go`:
```go
package dashboard

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestGetLastTag(t *testing.T) {
    server := setupTestServer(t)
    
    req := httptest.NewRequest("GET", "/api/v2/sprint/last-tag", nil)
    rec := httptest.NewRecorder()
    
    server.handleGetLastTag(rec, req)
    
    assert.Equal(t, http.StatusOK, rec.Code)
    
    var tag LastTagInfo
    err := json.Unmarshal(rec.Body.Bytes(), &tag)
    require.NoError(t, err)
    
    assert.NotEmpty(t, tag.Tag)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/dashboard -run TestGetLastTag -v`
Expected: FAIL - handleGetLastTag undefined

**Step 3: Add GetLastTag to GitHub client**

Add to `internal/github/client.go`:
```go
// GetLastTag returns the most recent tag/release
func (c *Client) GetLastTag(ctx context.Context) (*TagInfo, error) {
    output, err := c.ghNoRepo("api", "repos/"+c.Repo+"/releases/latest")
    if err != nil {
        // Fallback to tags if no releases
        output, err = c.ghNoRepo("api", "repos/"+c.Repo+"/tags?per_page=1")
        if err != nil {
            return nil, fmt.Errorf("getting last tag: %w", err)
        }
    }
    
    var tags []struct {
        Name   string `json:"name"`
        Commit struct {
            SHA string `json:"sha"`
        } `json:"commit"`
    }
    
    if err := json.Unmarshal([]byte(output), &tags); err != nil {
        return nil, fmt.Errorf("parsing tags: %w", err)
    }
    
    if len(tags) == 0 {
        return nil, nil
    }
    
    return &TagInfo{
        Tag: tags[0].Name,
        SHA: tags[0].Commit.SHA,
    }, nil
}

type TagInfo struct {
    Tag string `json:"tag"`
    SHA string `json:"sha"`
}
```

**Step 4: Implement endpoint**

Create `internal/dashboard/api_v2_sprint.go`:
```go
package dashboard

import (
    "encoding/json"
    "net/http"
)

type LastTagInfo struct {
    Tag   string `json:"tag"`
    Date  string `json:"date,omitempty"`
    SHA   string `json:"sha"`
}

func (s *Server) handleGetLastTag(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    tagInfo, err := s.githubClient.GetLastTag(ctx)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    if tagInfo == nil {
        http.Error(w, "No tags found", http.StatusNotFound)
        return
    }
    
    info := LastTagInfo{
        Tag: tagInfo.Tag,
        SHA: tagInfo.SHA,
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(info)
}
```

**Step 5: Add route**

Modify `internal/dashboard/server.go` - add in setupRoutes():
```go
mux.HandleFunc("/api/v2/sprint/last-tag", s.handleGetLastTag)
```

**Step 6: Run test to verify it passes**

Run: `go test ./internal/dashboard -run TestGetLastTag -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/dashboard/api_v2_sprint.go internal/dashboard/api_v2_sprint_test.go internal/github/client.go internal/dashboard/server.go
git commit -m "feat: add GET /api/v2/sprint/last-tag endpoint"
```

---

## Task 3: Add GitHub Linked Issues Support

**Files:**
- Modify: `internal/github/client.go` (add GetLinkedIssues method)
- Test: `internal/github/client_test.go`

**Step 1: Write the failing test**

Add to `internal/github/client_test.go`:
```go
func TestClient_GetLinkedIssues(t *testing.T) {
    client := setupTestClient(t)
    
    linked, err := client.GetLinkedIssues(context.Background(), 123)
    // May fail if API not available, but should not panic
    assert.NoError(t, err)
    // If API available, should return slice (may be empty)
    assert.NotNil(t, linked)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/github -run TestClient_GetLinkedIssues -v`
Expected: FAIL - GetLinkedIssues undefined

**Step 3: Implement GetLinkedIssues**

Add to `internal/github/client.go`:
```go
// LinkedIssue represents a linked issue relationship
type LinkedIssue struct {
    Number       int    `json:"number"`
    Relationship string `json:"relationship"` // blocked_by, blocks, relates_to
}

// GetLinkedIssues fetches linked issues using GitHub GraphQL API
// Falls back to parsing issue body if GraphQL not available
func (c *Client) GetLinkedIssues(ctx context.Context, issueNumber int) ([]LinkedIssue, error) {
    // Try GraphQL API first (GitHub Linked Issues)
    linked, err := c.getLinkedIssuesGraphQL(ctx, issueNumber)
    if err == nil {
        return linked, nil
    }
    
    // Fallback: parse issue body for #123 references
    return c.getLinkedIssuesFromBody(ctx, issueNumber)
}

func (c *Client) getLinkedIssuesGraphQL(ctx context.Context, issueNumber int) ([]LinkedIssue, error) {
    // GraphQL query for linked issues
    query := fmt.Sprintf(`{
        repository(owner: "%s", name: "%s") {
            issue(number: %d) {
                trackedIssues(first: 100) {
                    nodes {
                        number
                    }
                }
                trackedInIssues(first: 100) {
                    nodes {
                        number
                    }
                }
            }
        }
    }`, c.Owner, c.Repo, issueNumber)
    
    // Execute GraphQL query using gh api graphql
    output, err := c.ghNoRepo("api", "graphql", "-f", "query="+query)
    if err != nil {
        return nil, err
    }
    
    // Parse response
    var result struct {
        Data struct {
            Repository struct {
                Issue struct {
                    TrackedIssues struct {
                        Nodes []struct {
                            Number int `json:"number"`
                        } `json:"nodes"`
                    } `json:"trackedIssues"`
                    TrackedInIssues struct {
                        Nodes []struct {
                            Number int `json:"number"`
                        } `json:"nodes"`
                    } `json:"trackedInIssues"`
                } `json:"issue"`
            } `json:"repository"`
        } `json:"data"`
    }
    
    if err := json.Unmarshal([]byte(output), &result); err != nil {
        return nil, err
    }
    
    var linked []LinkedIssue
    for _, node := range result.Data.Repository.Issue.TrackedIssues.Nodes {
        linked = append(linked, LinkedIssue{
            Number:       node.Number,
            Relationship: "blocks",
        })
    }
    for _, node := range result.Data.Repository.Issue.TrackedInIssues.Nodes {
        linked = append(linked, LinkedIssue{
            Number:       node.Number,
            Relationship: "blocked_by",
        })
    }
    
    return linked, nil
}

func (c *Client) getLinkedIssuesFromBody(ctx context.Context, issueNumber int) ([]LinkedIssue, error) {
    // Get issue details
    issue, err := c.GetIssue(ctx, issueNumber)
    if err != nil {
        return nil, err
    }
    
    // Parse body for #123 references
    re := regexp.MustCompile(`#(\d+)`)
    matches := re.FindAllStringSubmatch(issue.Body, -1)
    
    var linked []LinkedIssue
    seen := make(map[int]bool)
    
    for _, match := range matches {
        if len(match) > 1 {
            num, _ := strconv.Atoi(match[1])
            if num > 0 && num != issueNumber && !seen[num] {
                seen[num] = true
                linked = append(linked, LinkedIssue{
                    Number:       num,
                    Relationship: "relates_to",
                })
            }
        }
    }
    
    return linked, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/github -run TestClient_GetLinkedIssues -v`
Expected: PASS (or SKIP if API not available in test env)

**Step 5: Commit**

```bash
git add internal/github/client.go internal/github/client_test.go
git commit -m "feat: add GitHub Linked Issues support with fallback"
```

---

## Task 4: Add GetUnassignedIssues Endpoint

**Files:**
- Modify: `internal/dashboard/api_v2_sprint.go`
- Test: `internal/dashboard/api_v2_sprint_test.go`

**Step 1: Write the failing test**

Add to `internal/dashboard/api_v2_sprint_test.go`:
```go
func TestGetUnassignedIssues(t *testing.T) {
    server := setupTestServer(t)
    
    req := httptest.NewRequest("GET", "/api/v2/issues/unassigned", nil)
    rec := httptest.NewRecorder()
    
    server.handleGetUnassignedIssues(rec, req)
    
    assert.Equal(t, http.StatusOK, rec.Code)
    
    var issues []IssueCandidate
    err := json.Unmarshal(rec.Body.Bytes(), &issues)
    require.NoError(t, err)
    
    // All returned issues should not have a milestone
    for _, issue := range issues {
        assert.NotZero(t, issue.Number)
        assert.NotEmpty(t, issue.Title)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/dashboard -run TestGetUnassignedIssues -v`
Expected: FAIL - handleGetUnassignedIssues undefined

**Step 3: Implement endpoint**

Add to `internal/dashboard/api_v2_sprint.go`:
```go
type IssueCandidate struct {
    Number     int      `json:"number"`
    Title      string   `json:"title"`
    Labels     []string `json:"labels"`
    Complexity *int     `json:"complexity,omitempty"`
}

func (s *Server) handleGetUnassignedIssues(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // Get all open issues without milestone
    issues, err := s.githubClient.ListIssuesWithoutMilestone(ctx)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    var candidates []IssueCandidate
    for _, issue := range issues {
        // Skip pull requests
        if issue.IsPullRequest {
            continue
        }
        
        candidate := IssueCandidate{
            Number: issue.Number,
            Title:  issue.Title,
            Labels: issue.Labels,
        }
        
        // Extract complexity from labels if present
        for _, label := range issue.Labels {
            if strings.HasPrefix(label, "complexity:") {
                if val, err := strconv.Atoi(strings.TrimPrefix(label, "complexity:")); err == nil {
                    candidate.Complexity = &val
                }
            }
        }
        
        candidates = append(candidates, candidate)
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(candidates)
}
```

**Step 4: Add route**

Modify `internal/dashboard/server.go` - add in setupRoutes():
```go
mux.HandleFunc("/api/v2/issues/unassigned", s.handleGetUnassignedIssues)
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/dashboard -run TestGetUnassignedIssues -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/dashboard/api_v2_sprint.go internal/dashboard/api_v2_sprint_test.go internal/dashboard/server.go
git commit -m "feat: add GET /api/v2/issues/unassigned endpoint"
```

---

## Task 5: Add AI Proposal Generation with Dependencies

**Files:**
- Create: `internal/scheduler/plan_proposer.go`
- Create: `internal/scheduler/plan_proposer_test.go`
- Modify: `internal/dashboard/api_v2_sprint.go`

**Step 1: Write the failing test**

Create `internal/scheduler/plan_proposer_test.go`:
```go
package scheduler

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestPlanProposer_CreateProposal(t *testing.T) {
    proposer := NewPlanProposer(nil, nil)
    
    candidates := []IssueCandidate{
        {Number: 1, Title: "Bug fix", Labels: []string{"priority:high", "type:bug"}},
        {Number: 2, Title: "Feature", Labels: []string{"priority:medium", "type:feature"}},
    }
    
    jobID, err := proposer.CreateProposal(candidates, 1, "v1.0.0")
    assert.NoError(t, err)
    assert.NotEmpty(t, jobID)
    
    // Check job exists
    status, err := proposer.GetProposalStatus(jobID)
    assert.NoError(t, err)
    assert.NotNil(t, status)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/scheduler -run TestPlanProposer_CreateProposal -v`
Expected: FAIL - PlanProposer undefined

**Step 3: Implement PlanProposer with dependency tree building**

Create `internal/scheduler/plan_proposer.go`:
```go
package scheduler

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "github.com/crazy-goat/one-dev-army/internal/github"
    "github.com/crazy-goat/one-dev-army/internal/llm"
    "github.com/google/uuid"
)

type IssueCandidate struct {
    Number     int      `json:"number"`
    Title      string   `json:"title"`
    Labels     []string `json:"labels"`
    Complexity *int     `json:"complexity,omitempty"`
}

type LinkedIssue struct {
    Number       int    `json:"number"`
    Relationship string `json:"relationship"`
}

type Branch struct {
    ID              string `json:"id"`
    Name            string `json:"name"`
    RootIssue       int    `json:"root_issue"`
    Issues          []int  `json:"issues"`
    TotalComplexity int    `json:"total_complexity"`
}

type ProposedIssue struct {
    IssueCandidate
    Reason       string `json:"reason"`
    Dependencies []int  `json:"dependencies"`
    Branch       string `json:"branch"`
}

type ProposalJob struct {
    ID        string          `json:"id"`
    Status    string          `json:"status"` // pending, processing, completed, failed
    Proposal  []ProposedIssue `json:"proposal,omitempty"`
    Branches  []Branch        `json:"branches,omitempty"`
    Error     string          `json:"error,omitempty"`
    CreatedAt time.Time       `json:"created_at"`
}

type PlanProposer struct {
    llmRouter    llm.Router
    githubClient *github.Client
    jobs         map[string]*ProposalJob
    mu           sync.RWMutex
}

func NewPlanProposer(router llm.Router, ghClient *github.Client) *PlanProposer {
    return &PlanProposer{
        llmRouter:    router,
        githubClient: ghClient,
        jobs:         make(map[string]*ProposalJob),
    }
}

func (p *PlanProposer) CreateProposal(candidates []IssueCandidate, targetCount int, lastTag string) (string, error) {
    jobID := uuid.New().String()
    
    job := &ProposalJob{
        ID:        jobID,
        Status:    "pending",
        CreatedAt: time.Now(),
    }
    
    p.mu.Lock()
    p.jobs[jobID] = job
    p.mu.Unlock()
    
    // Start async processing
    go p.processProposal(jobID, candidates, targetCount, lastTag)
    
    return jobID, nil
}

func (p *PlanProposer) processProposal(jobID string, candidates []IssueCandidate, targetCount int, lastTag string) {
    p.mu.Lock()
    job := p.jobs[jobID]
    job.Status = "processing"
    p.mu.Unlock()
    
    // Build dependency graph
    ctx := context.Background()
    graph := p.buildDependencyGraph(ctx, candidates)
    
    // Build prompt for LLM
    prompt := buildProposalPrompt(candidates, targetCount, lastTag, graph)
    
    // Call LLM
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    
    response, err := p.llmRouter.Route(ctx, "planning", prompt)
    if err != nil {
        p.mu.Lock()
        job.Status = "failed"
        job.Error = err.Error()
        p.mu.Unlock()
        return
    }
    
    // Parse response
    var result struct {
        Issues   []ProposedIssue `json:"issues"`
        Branches []Branch        `json:"branches"`
    }
    if err := json.Unmarshal([]byte(response), &result); err != nil {
        p.mu.Lock()
        job.Status = "failed"
        job.Error = fmt.Sprintf("Failed to parse proposal: %v", err)
        p.mu.Unlock()
        return
    }
    
    p.mu.Lock()
    job.Status = "completed"
    job.Proposal = result.Issues
    job.Branches = result.Branches
    p.mu.Unlock()
}

func (p *PlanProposer) buildDependencyGraph(ctx context.Context, candidates []IssueCandidate) map[int][]LinkedIssue {
    graph := make(map[int][]LinkedIssue)
    
    for _, candidate := range candidates {
        linked, err := p.githubClient.GetLinkedIssues(ctx, candidate.Number)
        if err != nil {
            continue // Skip on error
        }
        graph[candidate.Number] = linked
    }
    
    return graph
}

func (p *PlanProposer) GetProposalStatus(jobID string) (*ProposalJob, error) {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    job, exists := p.jobs[jobID]
    if !exists {
        return nil, fmt.Errorf("job not found: %s", jobID)
    }
    
    return job, nil
}

func buildProposalPrompt(candidates []IssueCandidate, targetCount int, lastTag string, graph map[int][]LinkedIssue) string {
    candidatesJSON, _ := json.Marshal(candidates)
    graphJSON, _ := json.Marshal(graph)
    
    return fmt.Sprintf(`Given:
1. List of unassigned GitHub issues (with labels, complexity):
%s

2. Last release tag: %s (for context)

3. Target count: %d (soft limit, can exceed by up to 20%%)

4. Dependency graph (issue number -> linked issues):
%s

Select the best issues to include in the current sprint.

Rules:
- Group issues into branches based on dependencies (transitive closure)
- Select complete branches only (all or nothing)
- Prioritize: priority:high > priority:medium > priority:low
- Consider what was done in last release for context
- Can exceed target by up to 20%% if it completes important branches
- Return 100-120%% of target count

Return JSON with:
1. Selected issues with reasoning and branch assignment
2. Branches structure (root issue + all dependencies in branch)

Format:
{
  "issues": [
    {
      "number": 123,
      "title": "Issue title",
      "labels": ["priority:high", "type:bug"],
      "complexity": 3,
      "reason": "High priority bug",
      "dependencies": [456, 789],
      "branch": "auth-epic"
    }
  ],
  "branches": [
    {
      "id": "auth-epic",
      "name": "Epic: User Authentication",
      "root_issue": 123,
      "issues": [123, 456, 789],
      "total_complexity": 12
    }
  ]
}

Do not include any other text, only the JSON.`, candidatesJSON, lastTag, targetCount, graphJSON)
}
```

**Step 4: Add endpoint handlers**

Add to `internal/dashboard/api_v2_sprint.go`:
```go
type CreateProposalRequest struct {
    TargetCount int `json:"targetCount"`
}

type CreateProposalResponse struct {
    JobID string `json:"jobId"`
}

func (s *Server) handleCreateProposal(w http.ResponseWriter, r *http.Request) {
    var req CreateProposalRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // Get unassigned issues
    ctx := r.Context()
    issues, err := s.githubClient.ListIssuesWithoutMilestone(ctx)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    var candidates []scheduler.IssueCandidate
    for _, issue := range issues {
        if issue.IsPullRequest {
            continue
        }
        
        candidate := scheduler.IssueCandidate{
            Number: issue.Number,
            Title:  issue.Title,
            Labels: issue.Labels,
        }
        candidates = append(candidates, candidate)
    }
    
    // Get last tag for context
    lastTag, _ := s.githubClient.GetLastTag(ctx)
    lastTagName := ""
    if lastTag != nil {
        lastTagName = lastTag.Tag
    }
    
    // Create proposal job
    jobID, err := s.planProposer.CreateProposal(candidates, req.TargetCount, lastTagName)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(CreateProposalResponse{JobID: jobID})
}

func (s *Server) handleGetProposal(w http.ResponseWriter, r *http.Request) {
    jobID := r.PathValue("jobId")
    if jobID == "" {
        http.Error(w, "jobId required", http.StatusBadRequest)
        return
    }
    
    job, err := s.planProposer.GetProposalStatus(jobID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(job)
}
```

**Step 5: Add routes and initialize PlanProposer**

Modify `internal/dashboard/server.go`:
```go
type Server struct {
    // ... existing fields
    planProposer *scheduler.PlanProposer
}

// In NewServer(), add:
planProposer: scheduler.NewPlanProposer(llmRouter, githubClient),

// In setupRoutes(), add:
mux.HandleFunc("POST /api/v2/sprint/propose", s.handleCreateProposal)
mux.HandleFunc("GET /api/v2/sprint/propose/{jobId}", s.handleGetProposal)
```

**Step 6: Run test to verify it passes**

Run: `go test ./internal/scheduler -run TestPlanProposer -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/scheduler/plan_proposer.go internal/scheduler/plan_proposer_test.go internal/dashboard/api_v2_sprint.go internal/dashboard/server.go
git commit -m "feat: add AI proposal generation with dependency tree"
```

---

## Task 6: Add SSE Assignment Endpoint

**Files:**
- Modify: `internal/dashboard/api_v2_sprint.go`
- Test: `internal/dashboard/api_v2_sprint_test.go`

**Step 1: Write the failing test**

Add to `internal/dashboard/api_v2_sprint_test.go`:
```go
func TestAssignIssuesSSE(t *testing.T) {
    server := setupTestServer(t)
    
    req := httptest.NewRequest("POST", "/api/v2/sprint/assign", strings.NewReader(`{
        "issueNumbers": [1, 2],
        "branches": ["branch-1"]
    }`))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    
    server.handleAssignIssues(rec, req)
    
    assert.Equal(t, http.StatusOK, rec.Code)
    assert.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
    
    // Should receive progress events
    body := rec.Body.String()
    assert.Contains(t, body, "progress")
    assert.Contains(t, body, "completed")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/dashboard -run TestAssignIssuesSSE -v`
Expected: FAIL - handleAssignIssues undefined

**Step 3: Implement SSE endpoint**

Add to `internal/dashboard/api_v2_sprint.go`:
```go
type AssignRequest struct {
    IssueNumbers []int    `json:"issueNumbers"`
    Branches     []string `json:"branches,omitempty"`
}

type AssignProgress struct {
    Type    string `json:"type"` // progress, completed, error
    Current int    `json:"current"`
    Total   int    `json:"total"`
    Issue   int    `json:"issue,omitempty"`
    Branch  string `json:"branch,omitempty"`
    Error   string `json:"error,omitempty"`
}

func (s *Server) handleAssignIssues(w http.ResponseWriter, r *http.Request) {
    var req AssignRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // Get current sprint using existing SprintDetector
    ctx := r.Context()
    sprintDetector := github.NewSprintDetector(s.githubClient)
    currentSprint, err := sprintDetector.GetCurrentSprint()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    if currentSprint == nil {
        http.Error(w, "No active sprint", http.StatusBadRequest)
        return
    }
    
    // Setup SSE
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
    
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming not supported", http.StatusInternalServerError)
        return
    }
    
    total := len(req.IssueNumbers)
    
    // Map issue to branch for progress reporting
    issueToBranch := make(map[int]string)
    // TODO: Build this map from request data
    
    for i, issueNumber := range req.IssueNumbers {
        // Assign issue to milestone
        err := s.githubClient.SetIssueMilestone(ctx, issueNumber, currentSprint.Number)
        
        progress := AssignProgress{
            Type:    "progress",
            Current: i + 1,
            Total:   total,
            Issue:   issueNumber,
            Branch:  issueToBranch[issueNumber],
        }
        
        if err != nil {
            progress.Type = "error"
            progress.Error = err.Error()
        }
        
        // Send SSE event
        data, _ := json.Marshal(progress)
        fmt.Fprintf(w, "data: %s\n\n", data)
        flusher.Flush()
        
        // Small delay to avoid rate limiting
        time.Sleep(100 * time.Millisecond)
    }
    
    // Send completion event
    completed := AssignProgress{Type: "completed", Current: total, Total: total}
    data, _ := json.Marshal(completed)
    fmt.Fprintf(w, "data: %s\n\n", data)
    flusher.Flush()
}
```

**Step 4: Add route**

Modify `internal/dashboard/server.go`:
```go
mux.HandleFunc("POST /api/v2/sprint/assign", s.handleAssignIssues)
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/dashboard -run TestAssignIssuesSSE -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/dashboard/api_v2_sprint.go internal/dashboard/api_v2_sprint_test.go internal/dashboard/server.go
git commit -m "feat: add SSE endpoint for assigning issues to sprint"
```

---

## Task 7: Create React PlanSprintPage Component with Tree View

**Files:**
- Create: `web/src/pages/PlanSprintPage.tsx`
- Create: `web/src/pages/PlanSprintPage.test.tsx`
- Create: `web/src/types/sprint.ts`
- Create: `web/src/components/DependencyTree.tsx`
- Modify: `web/src/App.tsx` (add route)

**Step 1: Write the failing test**

Create `web/src/pages/PlanSprintPage.test.tsx`:
```tsx
import { render, screen } from '@testing-library/react';
import PlanSprintPage from './PlanSprintPage';

describe('PlanSprintPage', () => {
  it('renders sprint info', () => {
    render(<PlanSprintPage />);
    expect(screen.getByText(/Plan Sprint/i)).toBeInTheDocument();
  });

  it('shows target count input', () => {
    render(<PlanSprintPage />);
    expect(screen.getByLabelText(/Target ticket count/i)).toBeInTheDocument();
  });

  it('shows generate button', () => {
    render(<PlanSprintPage />);
    expect(screen.getByText(/Generate Proposal/i)).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd web && npm test -- PlanSprintPage.test.tsx`
Expected: FAIL - PlanSprintPage undefined

**Step 3: Create types**

Create `web/src/types/sprint.ts`:
```typescript
export interface Sprint {
  number: number;
  title: string;
  state: 'open' | 'closed';
  startDate: string;
  endDate: string;
  ticketCount: number;
}

export interface IssueCandidate {
  number: number;
  title: string;
  labels: string[];
  complexity?: number;
  priority?: 'high' | 'medium' | 'low';
  type?: 'bug' | 'feature' | 'docs' | 'other';
}

export interface Branch {
  id: string;
  name: string;
  root_issue: number;
  issues: number[];
  total_complexity: number;
}

export interface ProposedIssue extends IssueCandidate {
  reason: string;
  dependencies: number[];
  branch: string;
}

export interface ProposalJob {
  id: string;
  status: 'pending' | 'processing' | 'completed' | 'failed';
  proposal?: ProposedIssue[];
  branches?: Branch[];
  error?: string;
}

export interface AssignmentProgress {
  type: 'progress' | 'completed' | 'error';
  current: number;
  total: number;
  issue?: number;
  branch?: string;
  error?: string;
}

export interface TreeNode {
  issue: ProposedIssue;
  children: TreeNode[];
  branch: string;
}
```

**Step 4: Create DependencyTree component**

Create `web/src/components/DependencyTree.tsx`:
```tsx
import React from 'react';
import { TreeNode, Branch } from '../types/sprint';

interface DependencyTreeProps {
  nodes: TreeNode[];
  selectedIssues: Set<number>;
  onToggleBranch: (branchId: string, selected: boolean) => void;
}

const DependencyTree: React.FC<DependencyTreeProps> = ({ 
  nodes, 
  selectedIssues, 
  onToggleBranch 
}) => {
  const renderNode = (node: TreeNode, level: number = 0) => {
    const isSelected = selectedIssues.has(node.issue.number);
    const hasChildren = node.children.length > 0;
    const isRoot = level === 0;
    
    return (
      <div key={node.issue.number} className={`tree-node level-${level}`}>
        <label className={`tree-node-label ${isSelected ? 'selected' : ''}`}>
          <input
            type="checkbox"
            checked={isSelected}
            onChange={() => onToggleBranch(node.branch, !isSelected)}
          />
          <span className="issue-icon">{getTypeIcon(node.issue.labels)}</span>
          <div className="issue-details">
            <span className="issue-number">#{node.issue.number}</span>
            <span className="issue-title">{node.issue.title}</span>
            <div className="issue-meta">
              {node.issue.labels.map(label => (
                <span key={label} className="issue-label">{label}</span>
              ))}
              {node.issue.complexity && (
                <span className="issue-complexity">Complexity: {node.issue.complexity}</span>
              )}
            </div>
            <p className="issue-reason">{node.issue.reason}</p>
          </div>
        </label>
        {hasChildren && (
          <div className="tree-children">
            {node.children.map(child => renderNode(child, level + 1))}
          </div>
        )}
      </div>
    );
  };

  const getTypeIcon = (labels: string[]) => {
    if (labels.includes('type:bug')) return '🐛';
    if (labels.includes('type:feature')) return '✨';
    if (labels.includes('type:docs')) return '📚';
    return '📝';
  };

  return (
    <div className="dependency-tree">
      {nodes.map(node => renderNode(node))}
    </div>
  );
};

export default DependencyTree;
```

**Step 5: Implement PlanSprintPage component**

Create `web/src/pages/PlanSprintPage.tsx`:
```tsx
import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Sprint, ProposedIssue, ProposalJob, AssignmentProgress, TreeNode, Branch } from '../types/sprint';
import DependencyTree from '../components/DependencyTree';
import './PlanSprintPage.css';

const PlanSprintPage: React.FC = () => {
  const navigate = useNavigate();
  const [sprint, setSprint] = useState<Sprint | null>(null);
  const [targetCount, setTargetCount] = useState<number>(10);
  const [proposal, setProposal] = useState<ProposedIssue[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [selectedIssues, setSelectedIssues] = useState<Set<number>>(new Set());
  const [selectedBranches, setSelectedBranches] = useState<Set<string>>(new Set());
  const [isGenerating, setIsGenerating] = useState(false);
  const [isAssigning, setIsAssigning] = useState(false);
  const [assignmentProgress, setAssignmentProgress] = useState<AssignmentProgress | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Load current sprint on mount
  useEffect(() => {
    fetch('/api/v2/sprint/current')
      .then(res => {
        if (!res.ok) throw new Error('No active sprint');
        return res.json();
      })
      .then(setSprint)
      .catch(err => setError(err.message));
  }, []);

  const buildTree = (issues: ProposedIssue[], branches: Branch[]): TreeNode[] => {
    const issueMap = new Map(issues.map(i => [i.number, i]));
    const treeNodes: TreeNode[] = [];
    
    branches.forEach(branch => {
      const rootIssue = issueMap.get(branch.root_issue);
      if (!rootIssue) return;
      
      const buildBranchTree = (issueNum: number, branchId: string): TreeNode => {
        const issue = issueMap.get(issueNum);
        if (!issue) return null as any;
        
        const children = issue.dependencies
          .map(dep => buildBranchTree(dep, branchId))
          .filter(Boolean);
        
        return {
          issue,
          children,
          branch: branchId
        };
      };
      
      const rootNode = buildBranchTree(branch.root_issue, branch.id);
      if (rootNode) {
        treeNodes.push(rootNode);
      }
    });
    
    return treeNodes;
  };

  const handleGenerate = async () => {
    setIsGenerating(true);
    setError(null);
    
    try {
      // Start proposal generation
      const response = await fetch('/api/v2/sprint/propose', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ targetCount }),
      });
      
      if (!response.ok) throw new Error('Failed to start proposal');
      const { jobId } = await response.json();
      
      // Poll for completion
      const pollProposal = async (): Promise<{issues: ProposedIssue[], branches: Branch[]}> => {
        const statusRes = await fetch(`/api/v2/sprint/propose/${jobId}`);
        const job: ProposalJob = await statusRes.json();
        
        if (job.status === 'failed') {
          throw new Error(job.error || 'Proposal generation failed');
        }
        
        if (job.status === 'completed' && job.proposal && job.branches) {
          return { issues: job.proposal, branches: job.branches };
        }
        
        // Wait and retry
        await new Promise(resolve => setTimeout(resolve, 1000));
        return pollProposal();
      };
      
      const result = await pollProposal();
      setProposal(result.issues);
      setBranches(result.branches);
      
      // Select all by default
      const allIssueIds = new Set(result.issues.map(p => p.number));
      const allBranchIds = new Set(result.branches.map(b => b.id));
      setSelectedIssues(allIssueIds);
      setSelectedBranches(allBranchIds);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setIsGenerating(false);
    }
  };

  const handleToggleBranch = (branchId: string, selected: boolean) => {
    const branch = branches.find(b => b.id === branchId);
    if (!branch) return;
    
    setSelectedBranches(prev => {
      const next = new Set(prev);
      if (selected) {
        next.add(branchId);
      } else {
        next.delete(branchId);
      }
      return next;
    });
    
    setSelectedIssues(prev => {
      const next = new Set(prev);
      branch.issues.forEach(issueNum => {
        if (selected) {
          next.add(issueNum);
        } else {
          next.delete(issueNum);
        }
      });
      return next;
    });
  };

  const handleAssign = async () => {
    setIsAssigning(true);
    setError(null);
    
    const issueNumbers = Array.from(selectedIssues);
    const branchIds = Array.from(selectedBranches);
    
    try {
      const response = await fetch('/api/v2/sprint/assign', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ issueNumbers, branches: branchIds }),
      });
      
      if (!response.ok) throw new Error('Failed to start assignment');
      
      // Read SSE stream
      const reader = response.body?.getReader();
      const decoder = new TextDecoder();
      
      if (!reader) throw new Error('No response body');
      
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        
        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');
        
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data: AssignmentProgress = JSON.parse(line.slice(6));
            setAssignmentProgress(data);
            
            if (data.type === 'completed') {
              setTimeout(() => navigate('/'), 1000);
              return;
            }
            
            if (data.type === 'error') {
              throw new Error(data.error || 'Assignment failed');
            }
          }
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
      setIsAssigning(false);
    }
  };

  const treeNodes = buildTree(proposal, branches);
  const selectedCount = selectedIssues.size;
  const targetPercentage = Math.round((selectedCount / targetCount) * 100);
  const overcommit = targetPercentage > 100;

  if (error && !sprint) {
    return (
      <div className="plan-sprint-page">
        <div className="error-container">
          <h2>Error</h2>
          <p>{error}</p>
          <button onClick={() => navigate('/')}>Back to Board</button>
        </div>
      </div>
    );
  }

  return (
    <div className="plan-sprint-page">
      <header className="page-header">
        <button className="back-button" onClick={() => navigate('/')}>
          ← Back to Board
        </button>
        <h1>Plan Sprint</h1>
        {sprint && (
          <div className="sprint-info">
            <span className="sprint-title">{sprint.title}</span>
            <span className="sprint-dates">
              {new Date(sprint.startDate).toLocaleDateString()} - {new Date(sprint.endDate).toLocaleDateString()}
            </span>
          </div>
        )}
      </header>

      <main className="plan-content">
        <section className="config-section">
          <label htmlFor="target-count">Target ticket count:</label>
          <input
            id="target-count"
            type="number"
            min={1}
            max={50}
            value={targetCount}
            onChange={(e) => setTargetCount(parseInt(e.target.value) || 10)}
            disabled={isGenerating || isAssigning}
          />
          <button 
            onClick={handleGenerate} 
            disabled={isGenerating || isAssigning || !sprint}
            className="generate-button"
          >
            {isGenerating ? 'Generating...' : 'Generate Proposal'}
          </button>
        </section>

        {error && <div className="error-message">{error}</div>}

        {isGenerating && (
          <div className="loading-state">
            <div className="spinner" />
            <p>AI is selecting the best tickets for your sprint...</p>
            <p className="loading-subtitle">Analyzing dependencies and last release context</p>
          </div>
        )}

        {proposal.length > 0 && !isGenerating && (
          <section className="proposal-section">
            <h2>Proposed Tickets</h2>
            <p className="proposal-hint">
              Branches are selected as a whole. Unchecking any issue removes the entire branch.
            </p>
            <DependencyTree 
              nodes={treeNodes}
              selectedIssues={selectedIssues}
              onToggleBranch={handleToggleBranch}
            />
          </section>
        )}

        {isAssigning && assignmentProgress && (
          <section className="assignment-progress">
            <h3>Assigning tickets to sprint...</h3>
            <div className="progress-bar">
              <div 
                className="progress-fill" 
                style={{ width: `${(assignmentProgress.current / assignmentProgress.total) * 100}%` }}
              />
            </div>
            <p>
              {assignmentProgress.current} / {assignmentProgress.total} tickets assigned
            </p>
            {assignmentProgress.branch && (
              <p className="current-branch">Processing branch: {assignmentProgress.branch}...</p>
            )}
          </section>
        )}

        {proposal.length > 0 && !isAssigning && (
          <section className="action-bar">
            <div className={`selection-stats ${overcommit ? 'overcommit' : ''}`}>
              Selected: {selectedCount} / {targetCount} tickets ({targetPercentage}%)
              {overcommit && <span className="overcommit-warning"> (Over target)</span>}
            </div>
            <button 
              onClick={handleAssign}
              disabled={selectedIssues.size === 0}
              className="assign-button"
            >
              Assign to Sprint
            </button>
          </section>
        )}
      </main>
    </div>
  );
};

export default PlanSprintPage;
```

**Step 6: Add route**

Modify `web/src/App.tsx`:
```tsx
import PlanSprintPage from './pages/PlanSprintPage';

// In routes:
<Route path="/sprint/plan" element={<PlanSprintPage />} />
```

**Step 7: Run test to verify it passes**

Run: `cd web && npm test -- PlanSprintPage.test.tsx --watchAll=false`
Expected: PASS

**Step 8: Commit**

```bash
git add web/src/pages/PlanSprintPage.tsx web/src/pages/PlanSprintPage.test.tsx web/src/types/sprint.ts web/src/components/DependencyTree.tsx web/src/App.tsx
git commit -m "feat: add PlanSprintPage with dependency tree"
```

---

## Task 8: Add CSS Styles

**Files:**
- Create: `web/src/pages/PlanSprintPage.css`
- Create: `web/src/components/DependencyTree.css`

**Step 1: Create PlanSprintPage styles**

Create `web/src/pages/PlanSprintPage.css`:
```css
.plan-sprint-page {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px;
}

.page-header {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-bottom: 30px;
  padding-bottom: 20px;
  border-bottom: 1px solid #e0e0e0;
}

.back-button {
  padding: 8px 16px;
  background: #f5f5f5;
  border: 1px solid #ddd;
  border-radius: 4px;
  cursor: pointer;
}

.back-button:hover {
  background: #e0e0e0;
}

.page-header h1 {
  margin: 0;
  flex: 1;
}

.sprint-info {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}

.sprint-title {
  font-weight: bold;
  font-size: 1.1em;
}

.sprint-dates {
  color: #666;
  font-size: 0.9em;
}

.config-section {
  display: flex;
  align-items: center;
  gap: 15px;
  margin-bottom: 30px;
  padding: 20px;
  background: #f9f9f9;
  border-radius: 8px;
}

.config-section label {
  font-weight: 500;
}

.config-section input {
  width: 80px;
  padding: 8px;
  border: 1px solid #ddd;
  border-radius: 4px;
}

.generate-button {
  padding: 10px 20px;
  background: #4CAF50;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-weight: 500;
}

.generate-button:hover:not(:disabled) {
  background: #45a049;
}

.generate-button:disabled {
  background: #ccc;
  cursor: not-allowed;
}

.loading-state {
  text-align: center;
  padding: 40px;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 4px solid #f3f3f3;
  border-top: 4px solid #4CAF50;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 20px;
}

@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}

.loading-subtitle {
  color: #666;
  font-size: 0.9em;
}

.error-message {
  background: #ffebee;
  color: #c62828;
  padding: 15px;
  border-radius: 4px;
  margin-bottom: 20px;
}

.error-container {
  text-align: center;
  padding: 60px 20px;
}

.proposal-section h2 {
  margin-bottom: 10px;
}

.proposal-hint {
  color: #666;
  font-size: 0.9em;
  margin-bottom: 20px;
  font-style: italic;
}

.assignment-progress {
  background: #f5f5f5;
  padding: 30px;
  border-radius: 8px;
  text-align: center;
  margin: 20px 0;
}

.progress-bar {
  width: 100%;
  height: 20px;
  background: #e0e0e0;
  border-radius: 10px;
  overflow: hidden;
  margin: 20px 0;
}

.progress-fill {
  height: 100%;
  background: #4CAF50;
  transition: width 0.3s ease;
}

.current-branch {
  color: #666;
  font-style: italic;
}

.action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px;
  background: #f9f9f9;
  border-radius: 8px;
  margin-top: 30px;
  position: sticky;
  bottom: 20px;
}

.selection-stats {
  font-weight: 500;
}

.selection-stats.overcommit {
  color: #ff9800;
}

.overcommit-warning {
  font-size: 0.9em;
  margin-left: 8px;
}

.assign-button {
  padding: 12px 30px;
  background: #2196F3;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-weight: 500;
  font-size: 1em;
}

.assign-button:hover:not(:disabled) {
  background: #1976d2;
}

.assign-button:disabled {
  background: #ccc;
  cursor: not-allowed;
}
```

**Step 2: Create DependencyTree styles**

Create `web/src/components/DependencyTree.css`:
```css
.dependency-tree {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tree-node {
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  background: white;
  margin-left: calc(var(--level, 0) * 20px);
}

.tree-node.level-0 {
  --level: 0;
  border-left: 4px solid #2196F3;
}

.tree-node.level-1 {
  --level: 1;
  margin-left: 20px;
  border-left: 3px solid #90caf9;
}

.tree-node.level-2 {
  --level: 2;
  margin-left: 40px;
  border-left: 2px solid #bbdefb;
}

.tree-node-label {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 15px;
  cursor: pointer;
  transition: background 0.2s;
}

.tree-node-label:hover {
  background: #f5f5f5;
}

.tree-node-label.selected {
  background: #e3f2fd;
}

.tree-node-label input {
  margin-top: 4px;
}

.issue-icon {
  font-size: 1.2em;
}

.issue-details {
  flex: 1;
}

.issue-number {
  color: #666;
  font-weight: 500;
  margin-right: 8px;
}

.issue-title {
  font-weight: 500;
}

.issue-meta {
  display: flex;
  gap: 8px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.issue-label {
  background: #e3f2fd;
  color: #1976d2;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 0.85em;
}

.issue-complexity {
  background: #fff3e0;
  color: #f57c00;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 0.85em;
}

.issue-reason {
  margin: 8px 0 0 0;
  color: #666;
  font-size: 0.9em;
  font-style: italic;
}

.tree-children {
  margin-top: 8px;
}
```

**Step 3: Commit**

```bash
git add web/src/pages/PlanSprintPage.css web/src/components/DependencyTree.css
git commit -m "feat: add PlanSprintPage and DependencyTree styles"
```

---

## Task 9: Add "Plan Sprint" Button to Board

**Files:**
- Modify: `web/src/pages/BoardPage.tsx` (or equivalent)
- Modify: `web/src/components/BoardHeader.tsx` (if exists)

**Step 1: Find board component**

Check existing board component location and add button when sprint has 0 tickets.

**Step 2: Add button**

Add to board header/toolbar:
```tsx
{sprint?.ticketCount === 0 && (
  <button 
    className="plan-sprint-button"
    onClick={() => navigate('/sprint/plan')}
  >
    Plan Sprint
  </button>
)}
```

**Step 3: Add styles**

```css
.plan-sprint-button {
  padding: 8px 16px;
  background: #4CAF50;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-weight: 500;
}

.plan-sprint-button:hover {
  background: #45a049;
}
```

**Step 4: Commit**

```bash
git add web/src/pages/BoardPage.tsx web/src/components/BoardHeader.tsx
git commit -m "feat: add Plan Sprint button to board"
```

---

## Task 10: Run All Tests

**Step 1: Run Go tests**

```bash
go test ./internal/config ./internal/scheduler ./internal/dashboard ./internal/github -v
```
Expected: All tests PASS

**Step 2: Run React tests**

```bash
cd web && npm test -- --watchAll=false
```
Expected: All tests PASS

**Step 3: Run linter**

```bash
golangci-lint run ./...
cd web && npm run lint
```
Expected: No errors

**Step 4: Commit**

```bash
git commit -m "test: all tests passing for sprint plan UI"
```

---

## Summary

This implementation adds:

1. **Backend:**
   - Config for sprint planning (target count, overcommit %)
   - 5 new API endpoints (current sprint, last tag, unassigned issues, AI proposal, SSE assignment)
   - GitHub Linked Issues API integration with fallback to parsing
   - Async AI proposal generation with dependency tree building
   - SSE streaming for assignment progress with branch info

2. **Frontend:**
   - New React page at `/sprint/plan`
   - DependencyTree component with recursive tree visualization
   - "All or nothing" branch selection logic
   - Real-time progress bar for GitHub assignment
   - "Plan Sprint" button on board (when sprint empty)

3. **Features:**
   - Configurable target ticket count (soft limit with 20% overcommit)
   - AI considers priority, type, dependencies, and last release context
   - Tree view with branches selected as a whole
   - Error handling with retry options
   - Fallback for GitHub Linked Issues API
