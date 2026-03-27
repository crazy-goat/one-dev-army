package cmd

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/sprint"
)

// SprintCleanupFlags holds all the flags for the sprint cleanup command
type SprintCleanupFlags struct {
	Sprint string
	Force  bool
	DryRun bool
}

// SprintCommand handles the 'sprint' subcommand and its children
func SprintCommand(args []string, dir string) error {
	if len(args) < 1 {
		return errors.New("sprint command requires a subcommand: cleanup")
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "cleanup":
		return SprintCleanupCommand(subArgs, dir)
	default:
		return fmt.Errorf("unknown sprint subcommand: %s", subcommand)
	}
}

// SprintCleanupCommand handles the 'sprint cleanup' subcommand
func SprintCleanupCommand(args []string, dir string) error {
	flags := SprintCleanupFlags{}

	// Create a new flag set for this subcommand
	fs := flag.NewFlagSet("sprint cleanup", flag.ContinueOnError)
	fs.StringVar(&flags.Sprint, "sprint", "", "Sprint name to cleanup (defaults to most recently closed)")
	fs.BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")
	fs.BoolVar(&flags.DryRun, "dry-run", false, "Preview what would be deleted without actually deleting")

	// Parse the flags
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Load config
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Create GitHub client
	gh := github.NewClient(cfg.GitHub.Repo)

	// Create branch manager
	branchMgr := git.NewBranchManager(dir)

	// Create cleanup manager
	manager := sprint.NewCleanupManager(dir, gh, branchMgr)

	// Detect artifacts
	fmt.Println("Detecting artifacts from completed sprint...")
	plan, err := manager.DetectArtifacts(flags.Sprint)
	if err != nil {
		return fmt.Errorf("detecting artifacts: %w", err)
	}

	// Display summary
	fmt.Printf("\nSprint: %s\n\n", plan.Sprint)

	// Display branches
	if len(plan.Branches) > 0 {
		fmt.Printf("Branches to cleanup (%d):\n", len(plan.Branches))
		for _, branch := range plan.Branches {
			status := "✓"
			if !branch.CanDelete {
				switch {
				case branch.HasUnmerged:
					status = "⚠ (unmerged changes)"
				case branch.HasOpenPR:
					status = "⚠ (open PR)"
				default:
					status = "⚠"
				}
			}
			fmt.Printf("  %s %s (issue #%d)\n", status, branch.Name, branch.IssueNumber)
		}
		fmt.Println()
	}

	// Display worktrees
	if len(plan.Worktrees) > 0 {
		fmt.Printf("Worktrees to cleanup (%d):\n", len(plan.Worktrees))
		for _, wt := range plan.Worktrees {
			fmt.Printf("  %s (branch: %s)\n", wt.Path, wt.Branch)
		}
		fmt.Println()
	}

	// Display temp files
	if len(plan.TempFiles) > 0 {
		fmt.Printf("Temp files to cleanup (%d):\n", len(plan.TempFiles))
		for _, tf := range plan.TempFiles {
			fmt.Printf("  %s\n", tf.Path)
		}
		fmt.Println()
	}

	// Count deletable items
	deletableBranches := 0
	for _, b := range plan.Branches {
		if b.CanDelete {
			deletableBranches++
		}
	}

	totalDeletable := deletableBranches + len(plan.Worktrees) + len(plan.TempFiles)

	if totalDeletable == 0 {
		fmt.Println("No artifacts to cleanup.")
		return nil
	}

	fmt.Printf("Total items to delete: %d\n", totalDeletable)
	fmt.Printf("  - Branches: %d\n", deletableBranches)
	fmt.Printf("  - Worktrees: %d\n", len(plan.Worktrees))
	fmt.Printf("  - Temp files: %d\n", len(plan.TempFiles))
	fmt.Println()

	// Confirmation prompt (unless --force or --dry-run)
	if !flags.Force && !flags.DryRun {
		confirmed, err := confirmCleanup()
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if !confirmed {
			fmt.Println("Cleanup canceled.")
			return nil
		}
	}

	// Execute cleanup
	if flags.DryRun {
		fmt.Println("DRY RUN - No changes will be made")
		fmt.Println()
	}

	result, err := manager.Execute(plan, flags.DryRun)
	if err != nil {
		return fmt.Errorf("executing cleanup: %w", err)
	}

	// Display results
	fmt.Println("Cleanup Results:")
	fmt.Printf("  ✓ Deleted branches: %d\n", result.DeletedBranches)
	fmt.Printf("  ✓ Removed worktrees: %d\n", result.DeletedWorktrees)
	fmt.Printf("  ✓ Deleted temp files: %d\n", result.DeletedTempFiles)

	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(result.Errors))
		for _, errMsg := range result.Errors {
			fmt.Printf("  ! %s\n", errMsg)
		}
	}

	if flags.DryRun {
		fmt.Println("\nThis was a dry run. Use --force to actually delete the artifacts.")
	}

	return nil
}

// confirmCleanup prompts the user for confirmation
func confirmCleanup() (bool, error) {
	fmt.Print("Proceed with cleanup? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}

// PrintSprintUsage prints usage information for the sprint command
func PrintSprintUsage() {
	fmt.Println("Usage: oda sprint <subcommand> [options]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  cleanup    Clean up artifacts from a completed sprint")
	fmt.Println()
	fmt.Println("Sprint Cleanup Options:")
	fmt.Println("  --sprint string    Sprint name to cleanup (defaults to most recently closed)")
	fmt.Println("  --force            Skip confirmation prompt")
	fmt.Println("  --dry-run          Preview what would be deleted without actually deleting")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  oda sprint cleanup                    # Cleanup most recent sprint")
	fmt.Println("  oda sprint cleanup --sprint \"Sprint 1\"  # Cleanup specific sprint")
	fmt.Println("  oda sprint cleanup --dry-run          # Preview what would be deleted")
	fmt.Println("  oda sprint cleanup --force            # Skip confirmation")
}
