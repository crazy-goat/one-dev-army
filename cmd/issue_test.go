package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
)

func TestValidateIssueCreateFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   IssueCreateFlags
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid flags - all required present",
			flags: IssueCreateFlags{
				Title: "Test Issue",
				Text:  "Test description",
			},
			wantErr: false,
		},
		{
			name: "missing title",
			flags: IssueCreateFlags{
				Title: "",
				Text:  "Test description",
			},
			wantErr: true,
			errMsg:  "missing required flags: --title",
		},
		{
			name: "missing text",
			flags: IssueCreateFlags{
				Title: "Test Issue",
				Text:  "",
			},
			wantErr: true,
			errMsg:  "missing required flags: --text",
		},
		{
			name: "missing both required",
			flags: IssueCreateFlags{
				Title: "",
				Text:  "",
			},
			wantErr: true,
			errMsg:  "missing required flags: --title, --text",
		},
		{
			name: "whitespace only title",
			flags: IssueCreateFlags{
				Title: "   ",
				Text:  "Test description",
			},
			wantErr: true,
			errMsg:  "missing required flags: --title",
		},
		{
			name: "valid with all optional flags",
			flags: IssueCreateFlags{
				Title:    "Test Issue",
				Text:     "Test description",
				Priority: "high",
				Size:     "M",
				Type:     "bug",
			},
			wantErr: false,
		},
		{
			name: "invalid priority",
			flags: IssueCreateFlags{
				Title:    "Test Issue",
				Text:     "Test description",
				Priority: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid priority",
		},
		{
			name: "invalid size",
			flags: IssueCreateFlags{
				Title: "Test Issue",
				Text:  "Test description",
				Size:  "XXL",
			},
			wantErr: true,
			errMsg:  "invalid size",
		},
		{
			name: "invalid type",
			flags: IssueCreateFlags{
				Title: "Test Issue",
				Text:  "Test description",
				Type:  "task",
			},
			wantErr: true,
			errMsg:  "invalid type",
		},
		{
			name: "valid priority values",
			flags: IssueCreateFlags{
				Title:    "Test Issue",
				Text:     "Test description",
				Priority: "low",
			},
			wantErr: false,
		},
		{
			name: "valid size values",
			flags: IssueCreateFlags{
				Title: "Test Issue",
				Text:  "Test description",
				Size:  "XL",
			},
			wantErr: false,
		},
		{
			name: "valid type values - feature",
			flags: IssueCreateFlags{
				Title: "Test Issue",
				Text:  "Test description",
				Type:  "feature",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIssueCreateFlags(tt.flags)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateIssueCreateFlags() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateIssueCreateFlags() error = %v, want error containing %v", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("validateIssueCreateFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIssueCreateCommand_Help(t *testing.T) {
	// Test that help flag works
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "help flag",
			args: []string{"--help"},
		},
		{
			name: "short help flag",
			args: []string{"-h"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Just verify it doesn't panic - help prints to stdout
			// We can't easily capture stdout in this test structure
			_ = IssueCreateCommand(tt.args, nil, 0)
		})
	}
}

func TestTriggerDashboardSync(t *testing.T) {
	tests := []struct {
		name             string
		serverResponse   int
		expectSyncCalled bool
	}{
		{
			name:             "successful sync",
			serverResponse:   http.StatusOK,
			expectSyncCalled: true,
		},
		{
			name:             "sync with non-200 status",
			serverResponse:   http.StatusInternalServerError,
			expectSyncCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/sync" && r.Method == http.MethodPost {
					syncCalled = true
				}
				w.WriteHeader(tt.serverResponse)
			}))
			defer server.Close()

			// Extract port from server URL (format: http://127.0.0.1:PORT)
			portStr := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
			portStr = strings.TrimPrefix(portStr, "http://[::]:")
			port := 0
			fmt.Sscanf(portStr, "%d", &port)

			triggerDashboardSync(port)

			if syncCalled != tt.expectSyncCalled {
				t.Errorf("sync endpoint called = %v, want %v", syncCalled, tt.expectSyncCalled)
			}
		})
	}
}

func TestTriggerDashboardSync_DashboardNotRunning(_ *testing.T) {
	// Use an unused port to simulate dashboard not running
	triggerDashboardSync(59999)
	// Should not panic or return error - function should return gracefully
}

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestIssueLogCommand_FlagParsing(t *testing.T) {
	store := openTestStore(t)

	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "positional argument",
			args:    []string{"42"},
			wantErr: false,
		},
		{
			name:    "issue flag",
			args:    []string{"--issue", "42"},
			wantErr: false,
		},
		{
			name:    "flag wins over positional",
			args:    []string{"--issue", "99", "42"},
			wantErr: false,
		},
		{
			name:    "missing issue number",
			args:    []string{},
			wantErr: true,
			errMsg:  "issue number is required",
		},
		{
			name:    "invalid positional argument",
			args:    []string{"abc"},
			wantErr: true,
			errMsg:  "invalid issue number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := IssueLogCommand(tt.args, store, &buf)
			if tt.wantErr {
				if err == nil {
					t.Errorf("IssueLogCommand() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("IssueLogCommand() error = %v, want containing %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("IssueLogCommand() unexpected error = %v", err)
			}
		})
	}
}

func TestIssueLogCommand_TableOutput(t *testing.T) {
	store := openTestStore(t)

	// Insert sample stage changes
	now := time.Now().UTC()
	err := store.SaveStageChange(42, "", "analysis", "initial stage", "system")
	if err != nil {
		t.Fatalf("saving stage change: %v", err)
	}
	err = store.SaveStageChange(42, "analysis", "planning", "analysis complete", "orchestrator")
	if err != nil {
		t.Fatalf("saving stage change: %v", err)
	}

	var buf bytes.Buffer
	err = IssueLogCommand([]string{"42"}, store, &buf)
	if err != nil {
		t.Fatalf("IssueLogCommand() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Stage history for issue #42") {
		t.Errorf("output missing header: %s", output)
	}
	if !strings.Contains(output, "TIME") {
		t.Errorf("output missing TIME column: %s", output)
	}
	if !strings.Contains(output, "FROM") {
		t.Errorf("output missing FROM column: %s", output)
	}
	if !strings.Contains(output, "TO") {
		t.Errorf("output missing TO column: %s", output)
	}
	if !strings.Contains(output, "—") {
		t.Errorf("output missing em-dash for empty from_stage: %s", output)
	}
	if !strings.Contains(output, "analysis") {
		t.Errorf("output missing 'analysis' stage: %s", output)
	}
	if !strings.Contains(output, "planning") {
		t.Errorf("output missing 'planning' stage: %s", output)
	}

	// Verify the output contains the expected time format
	if !strings.Contains(output, now.Format("2006-01-02")) {
		t.Errorf("output missing date: %s", output)
	}
}

func TestIssueLogCommand_JSONOutput(t *testing.T) {
	store := openTestStore(t)

	// Insert sample stage changes
	err := store.SaveStageChange(42, "", "analysis", "initial stage", "system")
	if err != nil {
		t.Fatalf("saving stage change: %v", err)
	}
	err = store.SaveStageChange(42, "analysis", "planning", "analysis complete", "orchestrator")
	if err != nil {
		t.Fatalf("saving stage change: %v", err)
	}

	var buf bytes.Buffer
	err = IssueLogCommand([]string{"--issue", "42", "--json"}, store, &buf)
	if err != nil {
		t.Fatalf("IssueLogCommand() error = %v", err)
	}

	var output issueLogOutput
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("unmarshaling JSON output: %v", err)
	}

	if output.IssueNumber != 42 {
		t.Errorf("issue_number = %d, want 42", output.IssueNumber)
	}
	if output.Total != 2 {
		t.Errorf("total = %d, want 2", output.Total)
	}
	if len(output.Entries) != 2 {
		t.Errorf("entries length = %d, want 2", len(output.Entries))
	}

	// Verify entries are in correct order (newest first)
	if output.Entries[0].ToStage != "planning" {
		t.Errorf("first entry to_stage = %s, want planning", output.Entries[0].ToStage)
	}
	if output.Entries[1].ToStage != "analysis" {
		t.Errorf("second entry to_stage = %s, want analysis", output.Entries[1].ToStage)
	}
}

func TestIssueLogCommand_Limit(t *testing.T) {
	store := openTestStore(t)

	// Insert 5 stage changes
	for i := range 5 {
		err := store.SaveStageChange(42, fmt.Sprintf("stage%d", i), fmt.Sprintf("stage%d", i+1), "test", "system")
		if err != nil {
			t.Fatalf("saving stage change: %v", err)
		}
	}

	var buf bytes.Buffer
	err := IssueLogCommand([]string{"--issue", "42", "--limit", "2", "--json"}, store, &buf)
	if err != nil {
		t.Fatalf("IssueLogCommand() error = %v", err)
	}

	var output issueLogOutput
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("unmarshaling JSON output: %v", err)
	}

	if output.Total != 2 {
		t.Errorf("total = %d, want 2", output.Total)
	}
	if len(output.Entries) != 2 {
		t.Errorf("entries length = %d, want 2", len(output.Entries))
	}
}

func TestIssueLogCommand_EmptyLedger(t *testing.T) {
	store := openTestStore(t)

	var buf bytes.Buffer
	err := IssueLogCommand([]string{"42"}, store, &buf)
	if err != nil {
		t.Fatalf("IssueLogCommand() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No stage changes found for issue #42") {
		t.Errorf("output missing empty message: %s", output)
	}
}

func TestIssueLogCommand_NilStore(t *testing.T) {
	var buf bytes.Buffer
	err := IssueLogCommand([]string{"42"}, nil, &buf)
	if err == nil {
		t.Fatal("IssueLogCommand() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "database not available") {
		t.Errorf("error = %v, want containing 'database not available'", err)
	}
}
