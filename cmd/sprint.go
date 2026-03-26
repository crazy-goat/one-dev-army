package cmd

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/sprint"
)

// SprintCleanupFlags holds all the flags for the sprint cleanup command
type SprintCleanupFlags struct {
	Sprint string
	DryRun bool
	Force  bool
}

// SprintCommand handles the 'sprint' subcommand and its children
func SprintCommand(args []string, client *github.Client, repoDir string) error {
	if len(args) < 1 {
		return errors.New("sprint command requires a subcommand: cleanup")
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "cleanup":
		return SprintCleanupCommand(subArgs, client, repoDir)
	default:
		return fmt.Errorf("unknown sprint subcommand: %s", subcommand)
	}
}

// SprintCleanupCommand handles the 'sprint cleanup' subcommand
func SprintCleanupCommand(args []string, client *github.Client, repoDir string) error {
	flags := SprintCleanupFlags{}

	// Create a new flag set for this subcommand
	fs := flag.NewFlagSet("sprint cleanup", flag.ContinueOnError)
	fs.StringVar(&flags.Sprint, "sprint", "", "Sprint/milestone name to clean up (default: most recently closed)")
	fs.BoolVar(&flags.DryRun, "dry-run", false, "Show what would be deleted without actually deleting")
	fs.BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")

	// Parse the flags
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Create branch manager
	branchMgr := git.NewBranchManager(repoDir)

	// Create cleanup manager
	mgr := sprint.NewCleanupManager(repoDir, client, branchMgr)

	// Detect artifacts
	fmt.Println("Detecting artifacts from completed sprint...")
	plan, err := mgr.DetectArtifacts(flags.Sprint)
	if err != nil {
		return fmt.Errorf("detecting artifacts: %w", err)
	}

	// Print the plan
	sprint.PrintPlan(plan, os.Stdout)

	// Check if there's anything to clean up
	totalArtifacts := len(plan.Branches) + len(plan.Worktrees) + len(plan.TempFiles)
	if totalArtifacts == 0 {
		fmt.Println("No artifacts found to clean up.")
		return nil
	}

	// Count deletable items
	deletableBranches := 0
	for _, b := range plan.Branches {
		if b.CanDelete {
			deletableBranches++
		}
	}

	// In dry-run mode, just show the plan and exit
	if flags.DryRun {
		fmt.Println("Dry run complete. No changes were made.")
		fmt.Printf("Would delete: %d branches, %d worktrees, %d temp files\n",
			deletableBranches, len(plan.Worktrees), len(plan.TempFiles))
		return nil
	}

	// Confirm before proceeding (unless --force)
	if !flags.Force {
		fmt.Printf("\nProceed with cleanup? This will delete %d branches, %d worktrees, and %d temp files. [y/N]: ",
			deletableBranches, len(plan.Worktrees), len(plan.TempFiles))

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cleanup canceled.")
			return nil
		}
	}

	// Execute the cleanup
	fmt.Println("\nExecuting cleanup...")
	result, err := mgr.Execute(plan, false)
	if err != nil {
		return fmt.Errorf("executing cleanup: %w", err)
	}

	// Print results
	sprint.PrintResult(result, os.Stdout)

	// Return error if there were any failures
	if len(result.Errors) > 0 {
		return fmt.Errorf("cleanup completed with %d errors", len(result.Errors))
	}

	fmt.Println("Cleanup completed successfully!")
	return nil
}

// PrintSprintUsage prints usage information for the sprint command
func PrintSprintUsage() {
	fmt.Println("Usage: oda sprint <subcommand> [options]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  cleanup    Clean up artifacts from a completed sprint")
	fmt.Println()
	fmt.Println("Sprint Cleanup Options:")
	fmt.Println("  --sprint string    Sprint/milestone name to clean up (default: most recently closed)")
	fmt.Println("  --dry-run          Show what would be deleted without actually deleting")
	fmt.Println("  --force            Skip confirmation prompt")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  oda sprint cleanup                          # Clean up most recent sprint")
	fmt.Println("  oda sprint cleanup --sprint \"Sprint 1\"        # Clean up specific sprint")
	fmt.Println("  oda sprint cleanup --dry-run                # Preview what would be deleted")
	fmt.Println("  oda sprint cleanup --force                  # Skip confirmation")
}
