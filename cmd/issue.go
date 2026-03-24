package cmd

import (
	"errors"
	"flag"
	"fmt"
	"strings"

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

// IssueCommand handles the 'issue' subcommand and its children
func IssueCommand(args []string, client *github.Client) error {
	if len(args) < 1 {
		return errors.New("issue command requires a subcommand: create")
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "create":
		return IssueCreateCommand(subArgs, client)
	default:
		return fmt.Errorf("unknown issue subcommand: %s", subcommand)
	}
}

// IssueCreateCommand handles the 'issue create' subcommand
func IssueCreateCommand(args []string, client *github.Client) error {
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

// PrintIssueUsage prints usage information for the issue command
func PrintIssueUsage() {
	fmt.Println("Usage: oda issue <subcommand> [options]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  create    Create a new GitHub issue")
	fmt.Println()
	fmt.Println("Issue Create Options:")
	fmt.Println("  --title string         Issue title (required)")
	fmt.Println("  --text string          Issue body text (required)")
	fmt.Println("  --priority string      Issue priority: high, medium, or low")
	fmt.Println("  --size string          Issue size: S, M, L, or XL")
	fmt.Println("  --type string          Issue type: bug or feature")
	fmt.Println("  --current-sprint       Assign issue to the current sprint")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  oda issue create --title \"Fix login bug\" --text \"Users cannot login\" --priority high --type bug --current-sprint")
}
