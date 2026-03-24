package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/crazy-goat/one-dev-army/cmd"
	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/dashboard"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/initialize"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/mvp"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/preflight"
	"github.com/crazy-goat/one-dev-army/internal/setup"
	"github.com/crazy-goat/one-dev-army/internal/skills"
	"github.com/crazy-goat/one-dev-army/internal/worker"
)

//go:embed skills/*
var skillsFS embed.FS

func main() {
	// Define flags
	var debugWebSocket bool
	var workingDir string
	flag.BoolVar(&debugWebSocket, "debug-websocket", false, "Enable WebSocket debug logging")
	flag.StringVar(&workingDir, "working-dir", ".", "Working directory for opencode (defaults to current directory)")

	// Custom usage message
	flag.Usage = printUsage

	// Parse flags
	flag.Parse()

	// Validate and convert working directory to absolute path
	absDir, err := filepath.Abs(workingDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolving working directory: %v\n", err)
		os.Exit(1)
	}

	// Check if directory exists and is accessible
	info, err := os.Stat(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: working directory does not exist or is not accessible: %s\n", absDir)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "error: working directory path is not a directory: %s\n", absDir)
		os.Exit(1)
	}

	// Log startup message with absolute path
	fmt.Printf("Starting opencode in: %s\n", absDir)

	// Get remaining args after flag parsing
	args := flag.Args()

	if len(args) > 0 {
		switch args[0] {
		case "init":
			if err := runInit(absDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "issue":
			if err := runIssue(args[1:], absDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--help", "-h", "help":
			printUsage()
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
			printUsage()
			os.Exit(1)
		}
	}

	if err := runServe(absDir, debugWebSocket); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("ODA stopped.")
}

func printUsage() {
	fmt.Println("Usage: oda [options] [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  (none)    Start the ODA agent and dashboard")
	fmt.Println("  init      Initialize a new ODA project in the current directory")
	fmt.Println("  issue     Manage GitHub issues (create, list, etc.)")
	fmt.Println("  help      Show this help message")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --debug-websocket    Enable WebSocket debug logging")
	fmt.Println("  --working-dir        Working directory for opencode (default: current directory)")
}

func runInit(dir string) error {
	if err := preflight.CheckGitRepo(dir); err != nil {
		return err
	}

	i := initialize.New(dir, nil)
	return i.Run()
}

func runIssue(args []string, dir string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		cmd.PrintIssueUsage()
		return nil
	}

	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	gh := github.NewClient(cfg.GitHub.Repo)
	return cmd.IssueCommand(args, gh)
}

func runServe(dir string, debugWebSocket bool) error {
	const opencodeURL = "http://localhost:4096"

	var err error
	var spawnedServer *opencode.Server

	// Deploy embedded skills to .opencode/skills/
	fmt.Println("Deploying opencode skills...")
	skillsSubFS, err := fs.Sub(skillsFS, "skills")
	if err != nil {
		return fmt.Errorf("creating skills sub filesystem: %w", err)
	}
	if err := skills.Deploy(dir, skillsSubFS); err != nil {
		return fmt.Errorf("deploying skills: %w", err)
	}
	fmt.Println("  ✓ skills deployed")

	fmt.Println("Running preflight checks...")
	results := preflight.RunAll(dir, opencodeURL, func(name string, index, total int, status string) {
		desc := preflight.GetCheckDescription(name)
		switch status {
		case "running":
			fmt.Printf("  [%d/%d] → %s — %s\n", index, total, name, desc)
		case "ok":
			fmt.Printf("  [%d/%d] ✓ %s\n", index, total, name)
		default:
			fmt.Printf("  [%d/%d] ✗ %s\n", index, total, name)
		}
	})
	allOK := true
	for _, r := range results {
		if !r.OK {
			allOK = false
		}
	}

	// If opencode is not reachable, try to auto-start it.
	if !allOK {
		opencodeOK := true
		for _, r := range results {
			if r.Name == "opencode" && !r.OK {
				opencodeOK = false
				break
			}
		}

		if !opencodeOK {
			// Check if opencode binary is installed before trying to start it.
			if err := preflight.CheckOpencodeInstalled(); err != nil {
				return err
			}

			fmt.Println("  → opencode not running, starting opencode serve...")
			spawnedServer, err = opencode.StartServer(opencodeURL, dir, 10*time.Second)
			if err != nil {
				return fmt.Errorf("auto-starting opencode serve: %w\n\n  Start manually: opencode serve", err)
			}
			fmt.Println("  ✓ opencode serve started")

			// Re-run preflight now that opencode is up.
			results = preflight.RunAll(dir, opencodeURL, func(name string, index, total int, status string) {
				desc := preflight.GetCheckDescription(name)
				switch status {
				case "running":
					fmt.Printf("  [%d/%d] → %s — %s\n", index, total, name, desc)
				case "ok":
					fmt.Printf("  [%d/%d] ✓ %s\n", index, total, name)
				default:
					fmt.Printf("  [%d/%d] ✗ %s\n", index, total, name)
				}
			})
			allOK = true
			for _, r := range results {
				if !r.OK {
					allOK = false
				}
			}
		}
	}

	if !allOK {
		if spawnedServer != nil {
			_ = spawnedServer.Stop()
		}
		return errors.New("preflight checks failed")
	}
	fmt.Println()

	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	dbPath := filepath.Join(dir, ".oda", "metrics.db")
	store, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = store.Close() }()

	oc := opencode.NewClient(cfg.OpenCode.URL)
	oc.SetDirectory(dir)
	gh := github.NewClient(cfg.GitHub.Repo)

	fmt.Println("Validating configured models...")
	configModels := collectConfigModels(cfg)
	if err := oc.ValidateModels(configModels); err != nil {
		return err
	}
	fmt.Println("  ✓ all models available")
	fmt.Println()

	fmt.Println("Verifying GitHub setup...")
	if err := gh.EnsureLabels(); err != nil {
		return fmt.Errorf("ensuring labels: %w", err)
	}

	sprintTitle, err := gh.EnsureMilestone()
	if err != nil {
		return fmt.Errorf("ensuring milestone: %w", err)
	}
	if sprintTitle != "" {
		fmt.Printf("  ✓ sprint created (%s)\n", sprintTitle)
	} else {
		fmt.Println("  ✓ sprint found")
	}

	// Detect and set the active sprint (oldest open milestone)
	activeMilestone, err := gh.GetOldestOpenMilestone()
	if err != nil {
		return fmt.Errorf("detecting active sprint: %w", err)
	}
	if activeMilestone != nil {
		gh.SetActiveMilestone(activeMilestone)
		fmt.Printf("  ✓ active sprint: %s (due: %s)\n", activeMilestone.Title, activeMilestone.DueOn.Format("2006-01-02"))
	} else {
		fmt.Println("  ! no active sprint found")
	}

	// Conditionally setup GitHub Projects if enabled
	if cfg.GitHub.UseProjects {
		fmt.Println("  → GitHub Projects enabled, setting up project board...")
		if _, err := gh.EnsureProject("ODA"); err != nil { //nolint:staticcheck // deprecated but kept for backward compatibility
			return fmt.Errorf("ensuring project: %w", err)
		}
		fmt.Println("  ✓ project board ready")
	}

	// Step 4: Fetch all issues from GitHub for active milestone and populate cache
	if activeMilestone != nil {
		fmt.Printf("Fetching issues from GitHub for milestone: %s\n", activeMilestone.Title)
		issues, err := gh.ListIssuesForMilestone(activeMilestone.Title)
		if err != nil {
			fmt.Printf("  ! error fetching issues: %v\n", err)
			fmt.Println("  → continuing with empty cache")
		} else {
			// Step 5: Populate issue_cache table
			cachedCount := 0
			for _, issue := range issues {
				if err := store.SaveIssueCache(issue, activeMilestone.Title, true); err != nil {
					fmt.Printf("  ! error caching issue #%d: %v\n", issue.Number, err)
					continue
				}
				cachedCount++
			}
			fmt.Printf("  ✓ cached %d issues\n", cachedCount)
		}
	} else {
		fmt.Println("! no active milestone, skipping initial cache population")
	}

	// Start opencode web UI server (non-blocking - failure is logged but not fatal)
	fmt.Println("Starting opencode web UI...")
	webServer, err := dashboard.NewWebServer(cfg.OpenCode.WebPort, dir)
	if err != nil {
		fmt.Printf("  ! warning: invalid web server configuration: %v\n", err)
		fmt.Println("  → continuing without web UI")
	} else if err := webServer.Start(); err != nil {
		fmt.Printf("  ! warning: failed to start opencode web UI: %v\n", err)
		fmt.Println("  → continuing without web UI")
	} else {
		fmt.Printf("  ✓ opencode web UI started at %s\n", webServer.URL())
	}

	// Step 6: Create WebSocket hub
	fmt.Println("Creating WebSocket hub...")
	hub := dashboard.NewHub(debugWebSocket)
	go hub.Run()
	fmt.Println("  ✓ WebSocket hub started")

	// Step 7 & 8: Create sync service (will be started after orchestrator)
	fmt.Println("Creating sync service...")
	syncService := dashboard.NewSyncService(gh, store, hub, nil) // orchestrator will be set later
	if activeMilestone != nil {
		syncService.SetActiveMilestone(activeMilestone.Title)
	}
	fmt.Println("  ✓ Sync service created")

	// Create LLM router for multi-model configuration
	router := llm.NewRouter(&cfg.LLM)

	s := setup.New(dir, oc, cfg, router)
	if err := s.CheckAndGenerate(); err != nil {
		return fmt.Errorf("setup check failed: %w", err)
	}

	brMgr := git.NewBranchManager(dir)

	orchestrator := mvp.NewOrchestrator(cfg, gh, oc, brMgr, store, hub, router)

	// Auto-start sprint if configured
	if cfg.Sprint.AutoStart {
		fmt.Println("Auto-starting sprint (auto_start: true)...")
		orchestrator.Start()
		fmt.Println("  ✓ sprint auto-started")
	}

	processor := worker.NewProcessor(cfg, oc, gh, store, brMgr, router, orchestrator)

	// Set orchestrator in sync service
	syncService.SetOrchestrator(orchestrator)
	syncService.Start()
	fmt.Println("  ✓ Sync service started (30s interval)")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("Starting %d workers...\n", cfg.Workers.Count)
	pool := worker.NewPool(cfg.Workers.Count, &worker.EmptyQueue{}, processor)
	pool.Start(ctx)

	go func() {
		if err := orchestrator.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "orchestrator error: %v\n", err)
		}
	}()

	srv, err := dashboard.NewServer(cfg.Dashboard.Port, cfg.OpenCode.WebPort, store, pool.Workers, gh, orchestrator, oc, cfg.LLM.Planning.Model, hub, syncService, dir)
	if err != nil {
		return fmt.Errorf("creating dashboard server: %w", err)
	}

	fmt.Printf("Dashboard: http://localhost:%d\n", cfg.Dashboard.Port)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	srvErrCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErrCh <- err
		}
		close(srvErrCh)
	}()

	workersDone := make(chan struct{})
	go func() {
		pool.Wait()
		close(workersDone)
	}()

	select {
	case <-sigCh:
		fmt.Println("\nShutting down... Press Ctrl+C again to force quit.")
		cancel()
		orchestrator.Stop()

		go func() {
			<-sigCh
			fmt.Println("\nForce quitting...")
			os.Exit(1)
		}()

		<-workersDone
		fmt.Println("All workers finished.")

	case err := <-srvErrCh:
		return fmt.Errorf("dashboard server: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down dashboard: %w", err)
	}

	if spawnedServer != nil {
		fmt.Println("Stopping opencode serve...")
		if err := spawnedServer.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: stopping opencode serve: %v\n", err)
		}
	}

	// Stop opencode web UI server
	if webServer != nil {
		fmt.Println("Stopping opencode web UI...")
		if err := webServer.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: stopping opencode web UI: %v\n", err)
		}
	}

	return nil
}

func collectConfigModels(cfg *config.Config) []opencode.ModelRef {
	seen := make(map[string]bool)
	var models []opencode.ModelRef

	add := func(llm string) {
		if llm == "" || seen[llm] {
			return
		}
		seen[llm] = true
		models = append(models, opencode.ParseModelRef(llm))
	}

	// Add models from new LLM config (5 independent modes)
	add(cfg.LLM.Setup.Model)
	add(cfg.LLM.Planning.Model)
	add(cfg.LLM.Orchestration.Model)
	add(cfg.LLM.Code.Model)
	add(cfg.LLM.CodeHeavy.Model)

	return models
}
