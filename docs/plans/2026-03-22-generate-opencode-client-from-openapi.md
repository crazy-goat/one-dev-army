# Generate opencode HTTP client from OpenAPI spec

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Auto-generate the opencode HTTP client from the OpenAPI spec at `/doc` endpoint, replacing manual types while preserving domain helpers and SSE streaming logic.

**Architecture:** Use oapi-codegen v2 to generate typed client into `internal/opencode/gen/`, then refactor `client.go` to wrap the generated client with type aliases and domain-specific helpers. SSE event types remain manual since they're not in the OpenAPI spec.

**Tech Stack:** Go 1.25, oapi-codegen v2, OpenAPI 3.1

---

## Task 1: Add oapi-codegen dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum` (auto-generated)

**Step 1: Add oapi-codegen tool dependency**

Run: `go get github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`

**Step 2: Verify dependency added**

Run: `grep oapi-codegen go.mod`

Expected: Contains `github.com/oapi-codegen/oapi-codegen/v2`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add oapi-codegen v2 for OpenAPI client generation"
```

---

## Task 2: Fetch and commit OpenAPI spec snapshot

**Files:**
- Create: `api/opencode.yaml`

**Step 1: Fetch OpenAPI spec from running server**

Run: `curl -s http://localhost:8080/doc > api/opencode.yaml`

**Step 2: Verify spec is valid YAML**

Run: `head -20 api/opencode.yaml`

Expected: Shows `openapi: 3.1.0` or similar header

**Step 3: Commit**

```bash
git add api/opencode.yaml
git commit -m "feat: add OpenAPI spec snapshot for client generation"
```

---

## Task 3: Create oapi-codegen configuration

**Files:**
- Create: `internal/opencode/oapi-codegen.yaml`

**Step 1: Write generator config**

```yaml
package: gen
generate:
  client: true
  models: true
output: internal/opencode/gen/client.go
```

**Step 2: Commit**

```bash
git add internal/opencode/oapi-codegen.yaml
git commit -m "chore: add oapi-codegen configuration"
```

---

## Task 4: Add go:generate directive

**Files:**
- Create: `internal/opencode/generate.go`

**Step 1: Create generate.go with directive**

```go
package opencode

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=oapi-codegen.yaml ../../api/opencode.yaml
```

**Step 2: Verify directive works**

Run: `cd internal/opencode && go generate`

Expected: Creates `internal/opencode/gen/client.go` with generated types

**Step 3: Commit**

```bash
git add internal/opencode/generate.go
git commit -m "chore: add go:generate directive for client generation"
```

---

## Task 5: Generate initial client

**Files:**
- Create: `internal/opencode/gen/client.go`

**Step 1: Run code generation**

Run: `cd internal/opencode && go generate`

**Step 2: Verify generated file exists**

Run: `ls -la internal/opencode/gen/client.go`

Expected: File exists with generated types (Session, Message, Part, ModelRef, etc.)

**Step 3: Commit generated client**

```bash
git add internal/opencode/gen/
git commit -m "feat: generate opencode client from OpenAPI spec"
```

---

## Task 6: Refactor client.go to use generated types

**Files:**
- Modify: `internal/opencode/client.go`

**Step 1: Add import for generated package**

```go
import (
    // ... existing imports ...
    "github.com/crazy-goat/one-dev-army/internal/opencode/gen"
)
```

**Step 2: Add type aliases for backward compatibility**

Add after imports, before constants:

```go
// Type aliases for backward compatibility with generated types
type Session = gen.Session
type Message = gen.Message
type MessageInfo = gen.MessageInfo
type Part = gen.Part
type ModelRef = gen.ModelRef
type PermissionRule = gen.PermissionRule
type SendMessageRequest = gen.SendMessageRequest
type ProviderModel = gen.ProviderModel
type Provider = gen.Provider
type ProvidersResponse = gen.ProvidersResponse
```

**Step 3: Update Client struct to embed generated client**

Replace:
```go
type Client struct {
    baseURL    string
    directory  string
    httpClient *http.Client
}
```

With:
```go
type Client struct {
    baseURL    string
    directory  string
    httpClient *http.Client
    genClient  *gen.Client  // Generated client for typed operations
}
```

**Step 4: Update NewClient to initialize generated client**

Replace:
```go
func NewClient(baseURL string) *Client {
    return &Client{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: 10 * time.Minute,
        },
    }
}
```

With:
```go
func NewClient(baseURL string) *Client {
    httpClient := &http.Client{
        Timeout: 10 * time.Minute,
    }
    genClient, _ := gen.NewClient(baseURL, gen.WithHTTPClient(httpClient))
    
    return &Client{
        baseURL:    baseURL,
        httpClient: httpClient,
        genClient:  genClient,
    }
}
```

**Step 5: Update HealthCheck to use generated client**

Replace manual HTTP call with:
```go
func (c *Client) HealthCheck() error {
    resp, err := c.genClient.GetGlobalHealth(context.Background())
    if err != nil {
        return fmt.Errorf("health check request: %w", err)
    }
    
    if !resp.Healthy {
        return fmt.Errorf("health check: server reports unhealthy")
    }
    return nil
}
```

**Step 6: Update CreateSession to use generated client**

Replace manual HTTP call with:
```go
func (c *Client) CreateSession(title string) (*Session, error) {
    params := &gen.CreateSessionParams{}
    if c.directory != "" {
        params.Directory = &c.directory
    }
    
    req := gen.CreateSessionJSONRequestBody{
        Title:      title,
        Permission: allowAllPermissions,
    }
    
    resp, err := c.genClient.CreateSession(context.Background(), params, req)
    if err != nil {
        return nil, fmt.Errorf("create session: %w", err)
    }
    
    return &resp, nil
}
```

**Step 7: Update InitSession to use generated client**

Replace manual HTTP call with:
```go
func (c *Client) InitSession(sessionID string, model ModelRef) error {
    params := &gen.InitSessionParams{}
    if c.directory != "" {
        params.Directory = &c.directory
    }
    
    req := gen.InitSessionJSONRequestBody{
        ProviderID: model.ProviderID,
        ModelID:    model.ModelID,
        MessageID:  "msg-init-" + sessionID,
    }
    
    _, err := c.genClient.InitSession(context.Background(), sessionID, params, req)
    if err != nil {
        return fmt.Errorf("init session: %w", err)
    }
    return nil
}
```

**Step 8: Update SendMessageAsync to use generated client**

Replace manual HTTP call with:
```go
func (c *Client) SendMessageAsync(sessionID, prompt string, model ModelRef) error {
    req := gen.SendMessageAsyncJSONRequestBody{
        Parts: []gen.Part{{Type: "text", Text: prompt}},
        Model: &gen.ModelRef{
            ProviderID: model.ProviderID,
            ModelID:    model.ModelID,
        },
        System: systemPromptNoQuestions,
    }
    
    _, err := c.genClient.SendMessageAsync(context.Background(), sessionID, req)
    if err != nil {
        return fmt.Errorf("send message async: %w", err)
    }
    return nil
}
```

**Step 9: Update AbortSession to use generated client**

Replace manual HTTP call with:
```go
func (c *Client) AbortSession(sessionID string) error {
    _, err := c.genClient.AbortSession(context.Background(), sessionID)
    if err != nil {
        return fmt.Errorf("abort session: %w", err)
    }
    return nil
}
```

**Step 10: Update DeleteSession to use generated client**

Replace manual HTTP call with:
```go
func (c *Client) DeleteSession(sessionID string) error {
    params := &gen.DeleteSessionParams{}
    if c.directory != "" {
        params.Directory = &c.directory
    }
    
    _, err := c.genClient.DeleteSession(context.Background(), sessionID, params)
    if err != nil {
        return fmt.Errorf("delete session: %w", err)
    }
    return nil
}
```

**Step 11: Update GetMessages to use generated client**

Replace manual HTTP call with:
```go
func (c *Client) GetMessages(sessionID string) ([]Message, error) {
    resp, err := c.genClient.GetSessionMessages(context.Background(), sessionID)
    if err != nil {
        return nil, fmt.Errorf("get messages: %w", err)
    }
    return resp, nil
}
```

**Step 12: Update ListProviders to use generated client**

Replace manual HTTP call with:
```go
func (c *Client) ListProviders() (*ProvidersResponse, error) {
    resp, err := c.genClient.ListProviders(context.Background())
    if err != nil {
        return nil, fmt.Errorf("list providers: %w", err)
    }
    
    // Convert gen.ProvidersResponse to our alias type
    result := ProvidersResponse(*resp)
    return &result, nil
}
```

**Step 13: Run tests to verify refactoring**

Run: `go test ./internal/opencode/... -v`

Expected: All 12 tests pass

**Step 14: Commit**

```bash
git add internal/opencode/client.go
git commit -m "refactor: use generated client for all HTTP operations"
```

---

## Task 7: Update tests to use generated types

**Files:**
- Modify: `internal/opencode/client_test.go`

**Step 1: Update test imports**

Add import for gen package:
```go
import (
    // ... existing imports ...
    "github.com/crazy-goat/one-dev-army/internal/opencode/gen"
)
```

**Step 2: Update TestSendMessage to use gen types**

Replace:
```go
var receivedReq opencode.SendMessageRequest
```

With:
```go
var receivedReq gen.SendMessageRequest
```

**Step 3: Update TestSendMessageAsync to use gen types**

Replace:
```go
var req opencode.SendMessageRequest
```

With:
```go
var req gen.SendMessageRequest
```

**Step 4: Update TestGetMessages to use gen types**

Replace:
```go
json.NewEncoder(w).Encode([]opencode.Message{
    {
        Info:  opencode.MessageInfo{ID: "msg-1", SessionID: "sess-123", Role: "user"},
        Parts: []opencode.Part{{Type: "text", Text: "hello"}},
    },
```

With:
```go
json.NewEncoder(w).Encode([]gen.Message{
    {
        Info:  gen.MessageInfo{ID: "msg-1", SessionID: "sess-123", Role: "user"},
        Parts: []gen.Part{{Type: "text", Text: "hello"}},
    },
```

**Step 5: Run tests to verify changes**

Run: `go test ./internal/opencode/... -v`

Expected: All 12 tests pass

**Step 6: Commit**

```bash
git add internal/opencode/client_test.go
git commit -m "test: update tests to use generated types"
```

---

## Task 8: Add integration tests for generated client

**Files:**
- Create: `internal/opencode/gen/client_test.go`

**Step 1: Create integration test file**

```go
package gen_test

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/crazy-goat/one-dev-army/internal/opencode/gen"
)

func TestGeneratedClientHealthCheck(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/global/health" {
            t.Errorf("path = %q, want /global/health", r.URL.Path)
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]bool{"healthy": true})
    }))
    defer srv.Close()

    client, err := gen.NewClient(srv.URL)
    if err != nil {
        t.Fatalf("failed to create client: %v", err)
    }

    resp, err := client.GetGlobalHealth(context.Background())
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !resp.Healthy {
        t.Errorf("healthy = false, want true")
    }
}

func TestGeneratedClientCreateSession(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/session" {
            t.Errorf("path = %q, want /session", r.URL.Path)
        }
        if r.Method != http.MethodPost {
            t.Errorf("method = %q, want POST", r.Method)
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(gen.Session{
            ID:    "sess-gen-123",
            Title: "generated-test",
        })
    }))
    defer srv.Close()

    client, err := gen.NewClient(srv.URL)
    if err != nil {
        t.Fatalf("failed to create client: %v", err)
    }

    req := gen.CreateSessionJSONRequestBody{
        Title: "generated-test",
    }
    
    resp, err := client.CreateSession(context.Background(), &gen.CreateSessionParams{}, req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if resp.ID != "sess-gen-123" {
        t.Errorf("session.ID = %q, want %q", resp.ID, "sess-gen-123")
    }
}
```

**Step 2: Run integration tests**

Run: `go test ./internal/opencode/gen/... -v`

Expected: 2 new tests pass

**Step 3: Commit**

```bash
git add internal/opencode/gen/client_test.go
git commit -m "test: add integration tests for generated client"
```

---

## Task 9: Verify go generate works end-to-end

**Files:**
- None (verification only)

**Step 1: Clean generated file**

Run: `rm internal/opencode/gen/client.go`

**Step 2: Regenerate client**

Run: `cd internal/opencode && go generate`

**Step 3: Verify file regenerated**

Run: `ls -la internal/opencode/gen/client.go`

Expected: File exists

**Step 4: Run full test suite**

Run: `go test ./internal/opencode/... -v`

Expected: All 14 tests pass (12 original + 2 integration)

**Step 5: Verify build succeeds**

Run: `go build ./...`

Expected: No errors

**Step 6: Commit (if any changes)**

```bash
git add -A
git commit -m "chore: verify go generate workflow" || echo "No changes to commit"
```

---

## Task 10: Update AGENTS.md documentation

**Files:**
- Modify: `AGENTS.md`

**Step 1: Add section about client regeneration**

Add to AGENTS.md after the opencode section:

```markdown
### Regenerating the opencode client

The HTTP client in `internal/opencode/` is auto-generated from the OpenAPI spec:

```bash
# Regenerate client from OpenAPI spec
cd internal/opencode && go generate

# Or manually:
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
  --config=internal/opencode/oapi-codegen.yaml \
  api/opencode.yaml
```

**When to regenerate:**
- After updating `api/opencode.yaml` (OpenAPI spec snapshot)
- When opencode server API changes

**Files:**
- `api/opencode.yaml` - OpenAPI spec snapshot
- `internal/opencode/oapi-codegen.yaml` - Generator configuration
- `internal/opencode/gen/client.go` - Generated client (DO NOT EDIT)
- `internal/opencode/client.go` - Domain wrapper with helpers
```

**Step 2: Commit**

```bash
git add AGENTS.md
git commit -m "docs: document client regeneration workflow"
```

---

## Task 11: Final verification

**Files:**
- None (verification only)

**Step 1: Run all tests**

Run: `go test ./... -v`

Expected: All tests pass

**Step 2: Verify build**

Run: `go build ./...`

Expected: No errors

**Step 3: Check for lint issues**

Run: `go vet ./...`

Expected: No issues

**Step 4: Verify imports are clean**

Run: `go mod tidy`

Run: `git diff go.mod go.sum`

Expected: No unexpected changes

**Step 5: Final commit**

```bash
git add -A
git commit -m "feat: complete OpenAPI client generation (closes #6)" || echo "No changes to commit"
```

---

## Summary

This plan implements GitHub issue #6 by:

1. **Adding oapi-codegen v2** as a build dependency
2. **Committing OpenAPI spec** snapshot to `api/opencode.yaml`
3. **Generating typed client** into `internal/opencode/gen/`
4. **Refactoring client.go** to use generated types via aliases
5. **Preserving domain logic** (ParseModelRef, SSE streaming, ValidateModels)
6. **Updating tests** to use generated types
7. **Adding integration tests** for the generated client
8. **Supporting `go generate`** for easy regeneration
9. **Documenting** the workflow in AGENTS.md

**Key design decisions:**
- Type aliases maintain backward compatibility
- SSE event types remain manual (not in OpenAPI spec)
- Domain helpers stay in wrapper client
- Generated code is committed (not generated at build time)
