package opencode_test

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
)

func TestHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/global/health" {
			t.Errorf("path = %q, want /global/health", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"healthy": true})
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	if err := client.HealthCheck(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHealthCheckUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"healthy": false})
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	err := client.HealthCheck()
	if err == nil {
		t.Fatal("expected error for unhealthy server, got nil")
	}
}

func TestHealthCheckServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	err := client.HealthCheck()
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
}

func TestCreateSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session" {
			t.Errorf("path = %q, want /session", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshaling request: %v", err)
		}
		if req["title"] != "test-session" {
			t.Errorf("title = %q, want %q", req["title"], "test-session")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(opencode.Session{
			ID:    "sess-123",
			Title: "test-session",
		})
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	session, err := client.CreateSession("test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ID != "sess-123" {
		t.Errorf("session.ID = %q, want %q", session.ID, "sess-123")
	}
	if session.Title != "test-session" {
		t.Errorf("session.Title = %q, want %q", session.Title, "test-session")
	}
}

func TestSendMessage(t *testing.T) {
	var receivedReq opencode.SendMessageRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/event" && r.Method == http.MethodGet {
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "no flusher", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "data: %s\n\n", `{"type":"server.connected","properties":{}}`)
			flusher.Flush()
			<-r.Context().Done()
			return
		}

		if r.URL.Path == "/session/sess-123/prompt_async" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedReq)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	client.SendMessageStream(ctx, "sess-123", "hello world", opencode.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4"}, nil)

	if len(receivedReq.Parts) != 1 {
		t.Fatalf("parts length = %d, want 1", len(receivedReq.Parts))
	}
	if receivedReq.Parts[0].Text != "hello world" {
		t.Errorf("parts[0].text = %q, want %q", receivedReq.Parts[0].Text, "hello world")
	}
	if receivedReq.Model == nil || receivedReq.Model.ProviderID != "anthropic" || receivedReq.Model.ModelID != "claude-sonnet-4" {
		t.Errorf("model = %+v, want {anthropic claude-sonnet-4}", receivedReq.Model)
	}
}

func TestSendMessageAsync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/sess-123/prompt_async" {
			t.Errorf("path = %q, want /session/sess-123/prompt_async", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}

		var req opencode.SendMessageRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshaling request: %v", err)
		}
		if req.Parts[0].Text != "do something" {
			t.Errorf("parts[0].text = %q, want %q", req.Parts[0].Text, "do something")
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	if err := client.SendMessageAsync("sess-123", "do something", opencode.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAbortSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/sess-123/abort" {
			t.Errorf("path = %q, want /session/sess-123/abort", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	if err := client.AbortSession("sess-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/sess-123" {
			t.Errorf("path = %q, want /session/sess-123", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	if err := client.DeleteSession("sess-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/sess-123/message" {
			t.Errorf("path = %q, want /session/sess-123/message", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]opencode.Message{
			{
				Info:  opencode.MessageInfo{ID: "msg-1", SessionID: "sess-123", Role: "user"},
				Parts: []opencode.Part{{Type: "text", Text: "hello"}},
			},
			{
				Info:  opencode.MessageInfo{ID: "msg-2", SessionID: "sess-123", Role: "assistant"},
				Parts: []opencode.Part{{Type: "text", Text: "hi there"}},
			},
		})
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	messages, err := client.GetMessages("sess-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages length = %d, want 2", len(messages))
	}
	if messages[0].Info.Role != "user" {
		t.Errorf("messages[0].info.role = %q, want %q", messages[0].Info.Role, "user")
	}
	if messages[1].Info.Role != "assistant" {
		t.Errorf("messages[1].info.role = %q, want %q", messages[1].Info.Role, "assistant")
	}
	if messages[1].Parts[0].Text != "hi there" {
		t.Errorf("messages[1].parts[0].text = %q, want %q", messages[1].Parts[0].Text, "hi there")
	}
}

func TestSendMessageServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	_, err := client.SendMessage("sess-123", "hello", opencode.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4"}, nil)
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
}

func TestCreateSessionServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	client := opencode.NewClient(srv.URL)
	_, err := client.CreateSession("test")
	if err == nil {
		t.Fatal("expected error for 400 status, got nil")
	}
}

func TestParseModelRef(t *testing.T) {
	tests := []struct {
		input      string
		providerID string
		modelID    string
	}{
		{"claude-sonnet-4", "anthropic", "claude-sonnet-4"},
		{"claude-opus-4", "anthropic", "claude-opus-4"},
		{"gpt-4o", "openai", "gpt-4o"},
		{"o3-mini", "openai", "o3-mini"},
		{"gemini-2.5-pro", "google", "gemini-2.5-pro"},
		{"anthropic/claude-sonnet-4-20250514", "anthropic", "claude-sonnet-4-20250514"},
		{"openai/gpt-4o", "openai", "gpt-4o"},
		{"custom-model", "anthropic", "custom-model"},
		{"deepseek-r1", "deepseek", "deepseek-r1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ref := opencode.ParseModelRef(tt.input)
			if ref.ProviderID != tt.providerID {
				t.Errorf("ParseModelRef(%q).ProviderID = %q, want %q", tt.input, ref.ProviderID, tt.providerID)
			}
			if ref.ModelID != tt.modelID {
				t.Errorf("ParseModelRef(%q).ModelID = %q, want %q", tt.input, ref.ModelID, tt.modelID)
			}
		})
	}
}
