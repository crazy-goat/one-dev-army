package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
				if r.URL.Path == "/api/sync" && r.Method == "POST" {
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

func TestTriggerDashboardSync_DashboardNotRunning(t *testing.T) {
	// Use an unused port to simulate dashboard not running
	triggerDashboardSync(59999)
	// Should not panic or return error - function should return gracefully
}
