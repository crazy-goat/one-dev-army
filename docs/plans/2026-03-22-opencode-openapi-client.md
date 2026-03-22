# Generate opencode HTTP client from OpenAPI spec - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Auto-generate the opencode HTTP client from the OpenAPI 3.1 spec exposed by `opencode serve` at `/doc`, replacing manually maintained types while preserving domain-specific helpers.

**Architecture:** Use oapi-codegen to generate types and HTTP client from the OpenAPI spec into `internal/opencode/gen/`. The existing `client.go` will be refactored to import and wrap the generated client, keeping custom logic like SSE streaming and `ParseModelRef()`.

**Tech Stack:** Go 1.25, oapi-codegen v2, OpenAPI 3.1

---

## Task 1: Add oapi-codegen dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `tools.go`

**Step 1: Add oapi-codegen as direct dependency**

```bash
go get github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
go mod tidy
```

**Step 2: Create tools.go for go:generate**

```go
//go:build tools

package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
)
```

**Step 3: Verify installation**

```bash
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --version
```

Expected: Shows version number (e.g., v2.4.1)

**Step 4: Commit**

```bash
git add go.mod go.sum tools.go
git commit -m "deps: add oapi-codegen for OpenAPI client generation"
```

---

## Task 2: Fetch and commit OpenAPI spec

**Files:**
- Create: `api/opencode.yaml`

**Step 1: Start opencode serve and fetch spec**

```bash
# In one terminal, start the server
opencode serve &

# Fetch the OpenAPI spec
curl -s http://localhost:8080/doc > api/opencode.yaml
```

**Step 2: Verify spec is valid OpenAPI 3.1**

```bash
head -20 api/opencode.yaml
```

Expected: Shows `openapi: 3.1.0` or similar version header

**Step 3: Commit the spec snapshot**

```bash
git add api/opencode.yaml
git commit -m "feat: add OpenAPI spec snapshot from opencode serve"
```

---

## Task 3: Create oapi-codegen configuration

**Files:**
- Create: `internal/opencode/oapi-codegen.yaml`

**Step 1: Write oapi-codegen configuration**

```yaml
package: gen
generate:
  types: true
  client: true
  models: true
output: internal/opencode/gen/client.go
```

**Step 2: Create gen directory**

```bash
mkdir -p internal/opencode/gen
```

**Step 3: Commit configuration**

```bash
git add internal/opencode/oapi-codegen.yaml
git commit -m "chore: add oapi-codegen configuration"
```

---

## Task 4: Generate the client

**Files:**
- Create: `internal/opencode/gen/client.go`
- Create: `internal/opencode/generate.go`

**Step 1: Add go:generate directive**

```go
package opencode

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config oapi-codegen.yaml ../../api/opencode.yaml
```

**Step 2: Run code generation**

```bash
cd internal/opencode && go generate
```

**Step 3: Verify generated files**

```bash
ls -la internal/opencode/gen/
head -50 internal/opencode/gen/client.go
```

Expected: Generated file contains types like `Session`, `Message`, `Part`, `ModelRef`, etc.

**Step 4: Commit generated code**

```bash
git add internal/opencode/generate.go internal/opencode/gen/
git commit -m "feat: generate opencode client from OpenAPI spec"
```

---

## Task 5: Refactor client.go to use generated types

**Files:**
- Modify: `internal/opencode/client.go`

**Step 1: Add import for generated package**

```go
import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/opencode/gen"
)
```

**Step 2: Remove manual type definitions (lines 16-52)**

Delete these types (now in gen package):
- `Session`
- `Message`
- `MessageInfo`
- `Part`
- `ModelRef`
- `PermissionRule`
- `SendMessageRequest`

**Step 3: Remove manual type definitions (lines 231-267, 513-528)**

Delete these types (now in gen package):
- `sseEvent`
- `deltaProperties`
- `partUpdatedProperties`
- `messageUpdatedProperties`
- `sessionStatusProperties`
- `sessionIdleProperties`
- `ProviderModel`
- `Provider`
- `ProvidersResponse`

**Step 4: Add type aliases for backward compatibility**

```go
// Type aliases for backward compatibility with existing code
type Session = gen.Session
type Message = gen.Message
type MessageInfo = gen.MessageInfo
type Part = gen.Part
type ModelRef = gen.ModelRef
type SendMessageRequest = gen.SendMessageRequest
type ProviderModel = gen.ProviderModel
type Provider = gen.Provider
type ProvidersResponse = gen.ProvidersResponse
```

**Step 5: Update Client struct to embed generated client**

```go
type Client struct {
	baseURL       string
	directory     string
	httpClient    *http.Client
	genClient     *gen.Client  // Generated HTTP client
}
```

**Step 6: Update NewClient to initialize generated client**

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

**Step 7: Update HealthCheck to use generated client**

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

**Step 8: Update CreateSession to use generated client**

```go
func (c *Client) CreateSession(title string) (*Session, error) {
	req := gen.CreateSessionRequest{
		Title:      title,
		Permission: allowAllPermissions,
	}
	
	params := &gen.CreateSessionParams{}
	if c.directory != "" {
		params.Directory = &c.directory
	}
	
	session, err := c.genClient.CreateSession(context.Background(), req, params)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	
	return session, nil
}
```

**Step 9: Update InitSession to use generated client**

```go
func (c *Client) InitSession(sessionID string, model ModelRef) error {
	req := gen.InitSessionRequest{
		ProviderID: model.ProviderID,
		ModelID:    model.ModelID,
		MessageID:  "msg-init-" + sessionID,
	}
	
	params := &gen.InitSessionParams{}
	if c.directory != "" {
		params.Directory = &c.directory
	}
	
	_, err := c.genClient.InitSession(context.Background(), sessionID, req, params)
	if err != nil {
		return fmt.Errorf("init session: %w", err)
	}
	
	return nil
}
```

**Step 10: Update SendMessageAsync to use generated client**

```go
func (c *Client) SendMessageAsync(sessionID, prompt string, model ModelRef) error {
	req := gen.SendMessageRequest{
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

**Step 11: Keep SendMessageStream with SSE handling (custom logic)**

The SSE streaming logic in `SendMessageStream` (lines 273-435) is custom and not generated. Keep it as-is but update type references to use `gen.` prefix where needed.

**Step 12: Update AbortSession to use generated client**

```go
func (c *Client) AbortSession(sessionID string) error {
	_, err := c.genClient.AbortSession(context.Background(), sessionID)
	if err != nil {
		return fmt.Errorf("abort session: %w", err)
	}
	return nil
}
```

**Step 13: Update DeleteSession to use generated client**

```go
func (c *Client) DeleteSession(sessionID string) error {
	_, err := c.genClient.DeleteSession(context.Background(), sessionID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
```

**Step 14: Update GetMessages to use generated client**

```go
func (c *Client) GetMessages(sessionID string) ([]Message, error) {
	messages, err := c.genClient.GetSessionMessages(context.Background(), sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}
	return messages, nil
}
```

**Step 15: Update ListProviders to use generated client**

```go
func (c *Client) ListProviders() (*ProvidersResponse, error) {
	resp, err := c.genClient.ListProviders(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	return resp, nil
}
```

**Step 16: Keep ValidateModels and ParseModelRef (domain helpers)**

These functions (lines 74-87, 550-594) contain domain logic and should remain unchanged.

**Step 17: Run tests to verify refactoring**

```bash
cd /home/decodo/work/one-dev-army && go test ./internal/opencode/... -v
```

Expected: All tests pass

**Step 18: Commit**

```bash
git add internal/opencode/client.go
git commit -m "refactor: use generated client for HTTP operations"
```

---

## Task 6: Update tests for generated types

**Files:**
- Modify: `internal/opencode/client_test.go`

**Step 1: Update test to use gen package types**

Change imports to include gen package:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/opencode/gen"
)
```

**Step 2: Update TestCreateSession to use gen.Session**

```go
func TestCreateSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ... existing handler code ...
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gen.Session{
			ID:    "sess-123",
			Title: "test-session",
		})
	}))
	defer srv.Close()
	
	// ... rest of test ...
}
```

**Step 3: Update TestSendMessage to use gen.SendMessageRequest**

```go
func TestSendMessage(t *testing.T) {
	var receivedReq gen.SendMessageRequest
	
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ... existing handler code ...
		
		if r.URL.Path == "/session/sess-123/prompt_async" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedReq)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		
		// ... rest of handler ...
	}))
	defer srv.Close()
	
	// ... rest of test ...
}
```

**Step 4: Update TestSendMessageAsync to use gen.SendMessageRequest**

```go
func TestSendMessageAsync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ... existing handler code ...
		
		var req gen.SendMessageRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshaling request: %v", err)
		}
		
		// ... rest of test ...
	}))
	defer srv.Close()
	
	// ... rest of test ...
}
```

**Step 5: Update TestGetMessages to use gen.Message and gen.Part**

```go
func TestGetMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ... existing handler code ...
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]gen.Message{
			{
				Info:  gen.MessageInfo{ID: "msg-1", SessionID: "sess-123", Role: "user"},
				Parts: []gen.Part{{Type: "text", Text: "hello"}},
			},
			{
				Info:  gen.MessageInfo{ID: "msg-2", SessionID: "sess-123", Role: "assistant"},
				Parts: []gen.Part{{Type: "text", Text: "hi there"}},
			},
		})
	}))
	defer srv.Close()
	
	// ... rest of test ...
}
```

**Step 6: Run all tests**

```bash
cd /home/decodo/work/one-dev-army && go test ./internal/opencode/... -v
```

Expected: All 12 tests pass

**Step 7: Commit**

```bash
git add internal/opencode/client_test.go
git commit -m "test: update tests to use generated types"
```

---

## Task 7: Add integration test for generated client

**Files:**
- Create: `internal/opencode/gen/client_test.go`

**Step 1: Write integration test**

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
		t.Fatalf("creating client: %v", err)
	}

	resp, err := client.GetGlobalHealth(context.Background())
	if err != nil {
		t.Fatalf("health check: %v", err)
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
			ID:    "test-session-id",
			Title: "Test Session",
		})
	}))
	defer srv.Close()

	client, err := gen.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	req := gen.CreateSessionRequest{
		Title: "Test Session",
	}

	session, err := client.CreateSession(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.ID != "test-session-id" {
		t.Errorf("session.ID = %q, want %q", session.ID, "test-session-id")
	}
}
```

**Step 2: Run integration tests**

```bash
cd /home/decodo/work/one-dev-army && go test ./internal/opencode/gen/... -v
```

Expected: Tests pass

**Step 3: Commit**

```bash
git add internal/opencode/gen/client_test.go
git commit -m "test: add integration tests for generated client"
```

---

## Task 8: Verify go generate works end-to-end

**Files:**
- None (verification only)

**Step 1: Clean and regenerate**

```bash
rm -rf internal/opencode/gen/
cd internal/opencode && go generate
```

**Step 2: Verify code compiles**

```bash
cd /home/decodo/work/one-dev-army && go build ./...
```

Expected: No compilation errors

**Step 3: Run all tests**

```bash
cd /home/decodo/work/one-dev-army && go test ./internal/opencode/... -v
```

Expected: All tests pass

**Step 4: Document the generate command**

Add to README.md or AGENTS.md:

```markdown
## Regenerating the opencode client

To regenerate the HTTP client from the OpenAPI spec:

```bash
cd internal/opencode && go generate
```

This will regenerate `internal/opencode/gen/client.go` from `api/opencode.yaml`.
```

**Step 5: Commit documentation**

```bash
git add AGENTS.md
git commit -m "docs: document go generate command for client regeneration"
```

---

## Task 9: Final verification and cleanup

**Files:**
- None (verification only)

**Step 1: Run full test suite**

```bash
cd /home/decodo/work/one-dev-army && go test ./... -v
```

Expected: All tests pass

**Step 2: Verify no manual types remain**

```bash
grep -n "type Session struct" internal/opencode/client.go
grep -n "type Message struct" internal/opencode/client.go
grep -n "type Part struct" internal/opencode/client.go
grep -n "type ModelRef struct" internal/opencode/client.go
```

Expected: No matches (types are now in gen package)

**Step 3: Verify type aliases exist**

```bash
grep -n "type Session = gen.Session" internal/opencode/client.go
```

Expected: Shows the type alias

**Step 4: Final commit**

```bash
git commit --allow-empty -m "feat: complete OpenAPI client generation (closes #6)"
```

---

## Summary

This plan implements GitHub issue #6 by:

1. **Adding oapi-codegen** as a build dependency
2. **Fetching and committing** the OpenAPI spec from `opencode serve`
3. **Generating** a typed Go client into `internal/opencode/gen/`
4. **Refactoring** `client.go` to use the generated client while preserving:
   - Domain helpers (`ParseModelRef`, `ValidateModels`)
   - SSE streaming logic (`SendMessageStream`)
   - Backward compatibility via type aliases
5. **Updating tests** to use generated types
6. **Adding integration tests** for the generated client
7. **Documenting** the `go generate` workflow

**Acceptance Criteria Met:**
- [x] OpenAPI spec is committed as a snapshot
- [x] Go client is generated from the spec
- [x] All existing callers work with the generated client (via type aliases)
- [x] All tests pass
- [x] `go generate` command regenerates the client
