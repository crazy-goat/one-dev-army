package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

// IssueCreateFlags holds all the flags for the issue create command
type IssueCreateFlags struct {
	Title         string
	Text          string
	Priority      string
	Size          string
	Type          string
	CurrentSprint bool
}

// IssueLogFlags holds all the flags for the issue log command
type IssueLogFlags struct {
	Issue int
	JSON  bool
	Limit int
}

// IssueCommand handles the 'issue' subcommand and its children
func IssueCommand(args []string, client *github.Client, dashboardPort int, store *db.Store) error {
	if len(args) < 1 {
		return errors.New("issue command requires a subcommand: create, log")
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "create":
		return IssueCreateCommand(subArgs, client, dashboardPort)
	case "log":
		return IssueLogCommand(subArgs, store, os.Stdout)
	default:
		return fmt.Errorf("unknown issue subcommand: %s", subcommand)
	}
}

// IssueCreateCommand handles the 'issue create' subcommand
func IssueCreateCommand(args []string, client *github.Client, dashboardPort int) error {
	flags := IssueCreateFlags{}

	// Create a new flag set for this subcommand
	fs := flag.NewFlagSet("issue create", flag.ContinueOnError)
	fs.StringVar(&flags.Title, "title", "", "Issue title (required)")
	fs.StringVar(&flags.Text, "text", "", "Issue body text (required)")
	fs.StringVar(&flags.Priority, "priority", "", "Issue priority: high, medium, or low")
	fs.StringVar(&flags.Size, "size", "", "Issue size: S, M, L, or XL")
	fs.StringVar(&flags.Type, "type", "", "Issue type: bug or feature")
	fs.BoolVar(&flags.CurrentSprint, "current-sprint", false, "Assign to current sprint")

	// Parse the flags
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Validate required parameters
	if err := validateIssueCreateFlags(flags); err != nil {
		return err
	}

	// Build labels from parameters
	labels := github.BuildLabels(flags.Priority, flags.Size, flags.Type)

	// Determine milestone
	var milestone string
	if flags.CurrentSprint {
		sprintDetector := github.NewSprintDetector(client)
		sprintTitle, err := sprintDetector.GetCurrentSprintTitle()
		if err != nil {
			return fmt.Errorf("detecting current sprint: %w", err)
		}
		if sprintTitle == "" {
			return errors.New("no current sprint found (no open milestones)")
		}
		milestone = sprintTitle
	}

	// Create the issue
	var issueNum int
	var err error
	if milestone != "" {
		issueNum, err = client.CreateIssueWithMilestone(flags.Title, flags.Text, labels, milestone)
	} else {
		issueNum, err = client.CreateIssue(flags.Title, flags.Text, labels)
	}

	if err != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	fmt.Printf("Created issue #%d\n", issueNum)
	if milestone != "" {
		fmt.Printf("Assigned to sprint: %s\n", milestone)
	}
	if len(labels) > 0 {
		fmt.Printf("Labels: %s\n", strings.Join(labels, ", "))
	}

	// Trigger dashboard sync if port is configured
	if dashboardPort > 0 {
		triggerDashboardSync(dashboardPort)
	}

	return nil
}

// validateIssueCreateFlags validates that all required flags are present
func validateIssueCreateFlags(flags IssueCreateFlags) error {
	var missing []string

	if strings.TrimSpace(flags.Title) == "" {
		missing = append(missing, "--title")
	}

	if strings.TrimSpace(flags.Text) == "" {
		missing = append(missing, "--text")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}

	// Validate optional enum values
	if flags.Priority != "" {
		validPriorities := map[string]bool{"high": true, "medium": true, "low": true}
		if !validPriorities[flags.Priority] {
			return fmt.Errorf("invalid priority: %s (must be high, medium, or low)", flags.Priority)
		}
	}

	if flags.Size != "" {
		validSizes := map[string]bool{"S": true, "M": true, "L": true, "XL": true}
		if !validSizes[flags.Size] {
			return fmt.Errorf("invalid size: %s (must be S, M, L, or XL)", flags.Size)
		}
	}

	if flags.Type != "" {
		validTypes := map[string]bool{"bug": true, "feature": true}
		if !validTypes[flags.Type] {
			return fmt.Errorf("invalid type: %s (must be bug or feature)", flags.Type)
		}
	}

	return nil
}

// IssueLogCommand handles the 'issue log' subcommand
func IssueLogCommand(args []string, store *db.Store, w io.Writer) error {
	flags := IssueLogFlags{}

	fs := flag.NewFlagSet("issue log", flag.ContinueOnError)
	fs.IntVar(&flags.Issue, "issue", 0, "Issue number")
	fs.BoolVar(&flags.JSON, "json", false, "Output as JSON")
	fs.IntVar(&flags.Limit, "limit", 0, "Limit number of entries")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Positional argument (if --issue not set)
	if flags.Issue == 0 && fs.NArg() > 0 {
		n, err := strconv.Atoi(fs.Arg(0))
		if err != nil {
			return fmt.Errorf("invalid issue number: %s", fs.Arg(0))
		}
		flags.Issue = n
	}

	if flags.Issue <= 0 {
		return errors.New("issue number is required: oda issue log <number> or oda issue log --issue <number>")
	}

	if store == nil {
		return errors.New("database not available")
	}

	changes, err := store.GetStageChangesLimit(flags.Issue, flags.Limit)
	if err != nil {
		return fmt.Errorf("querying stage changes: %w", err)
	}

	if len(changes) == 0 {
		fmt.Fprintf(w, "No stage changes found for issue #%d\n", flags.Issue)
		return nil
	}

	if flags.JSON {
		return formatIssueLogJSON(w, flags.Issue, changes)
	}
	return formatIssueLogTable(w, flags.Issue, changes)
}

func formatIssueLogTable(w io.Writer, issueNumber int, changes []db.StageChange) error {
	fmt.Fprintf(w, "Stage history for issue #%d (%d entries)\n\n", issueNumber, len(changes))
	fmt.Fprintf(w, "%-21s %-13s %-13s %-25s %s\n", "TIME", "FROM", "TO", "REASON", "BY")

	for _, c := range changes {
		from := c.FromStage
		if from == "" {
			from = "—"
		}
		fmt.Fprintf(w, "%-21s %-13s %-13s %-25s %s\n",
			c.ChangedAt.Format("2006-01-02 15:04:05"),
			from,
			c.ToStage,
			c.Reason,
			c.ChangedBy,
		)
	}
	return nil
}

type issueLogOutput struct {
	IssueNumber int             `json:"issue_number"`
	Total       int             `json:"total"`
	Entries     []issueLogEntry `json:"entries"`
}

type issueLogEntry struct {
	ID        int    `json:"id"`
	FromStage string `json:"from_stage"`
	ToStage   string `json:"to_stage"`
	Reason    string `json:"reason"`
	ChangedBy string `json:"changed_by"`
	ChangedAt string `json:"changed_at"`
}

func formatIssueLogJSON(w io.Writer, issueNumber int, changes []db.StageChange) error {
	entries := make([]issueLogEntry, len(changes))
	for i, c := range changes {
		entries[i] = issueLogEntry{
			ID:        c.ID,
			FromStage: c.FromStage,
			ToStage:   c.ToStage,
			Reason:    c.Reason,
			ChangedBy: c.ChangedBy,
			ChangedAt: c.ChangedAt.Format(time.RFC3339),
		}
	}

	output := issueLogOutput{
		IssueNumber: issueNumber,
		Total:       len(entries),
		Entries:     entries,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// PrintIssueUsage prints usage information for the issue command
func PrintIssueUsage() {
	fmt.Println("Usage: oda issue <subcommand> [options]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  create    Create a new GitHub issue")
	fmt.Println("  log       Display stage change history for an issue")
	fmt.Println()
	fmt.Println("Issue Create Options:")
	fmt.Println("  --title string         Issue title (required)")
	fmt.Println("  --text string          Issue body text (required)")
	fmt.Println("  --priority string      Issue priority: high, medium, or low")
	fmt.Println("  --size string          Issue size: S, M, L, or XL")
	fmt.Println("  --type string          Issue type: bug or feature")
	fmt.Println("  --current-sprint       Assign issue to the current sprint")
	fmt.Println()
	fmt.Println("Issue Log Options:")
	fmt.Println("  --issue int            Issue number (alternative to positional argument)")
	fmt.Println("  --json                 Output as JSON instead of text table")
	fmt.Println("  --limit int            Limit number of entries (default: all, newest-first)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  oda issue create --title \"Fix login bug\" --text \"Users cannot login\" --priority high --type bug --current-sprint")
	fmt.Println("  oda issue log 42")
	fmt.Println("  oda issue log --issue 42 --json --limit 10")
}

// triggerDashboardSync sends an HTTP POST request to the dashboard sync endpoint
func triggerDashboardSync(port int) {
	url := fmt.Sprintf("http://localhost:%d/api/sync", port)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		log.Printf("Warning: dashboard sync failed (dashboard may not be running): %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("Dashboard synced")
	} else {
		log.Printf("Warning: dashboard sync returned status %d", resp.StatusCode)
	}
}
