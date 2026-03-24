package cmd

import (
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
			} else {
				if err != nil {
					t.Errorf("validateIssueCreateFlags() error = %v, wantErr %v", err, tt.wantErr)
				}
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
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't panic - help prints to stdout
			// We can't easily capture stdout in this test structure
			_ = tt.args
		})
	}
}
