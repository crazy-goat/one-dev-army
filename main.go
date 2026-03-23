package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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
	"github.com/crazy-goat/one-dev-army/internal/worker"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			if err := runInit(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--help", "-h", "help":
			printUsage()
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

	if err := runServe(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("ODA stopped.")
}

func printUsage() {
	fmt.Println("Usage: oda [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  (none)    Start the ODA agent and dashboard")
	fmt.Println("  init      Initialize a new ODA project in the current directory")
	fmt.Println("  help      Show this help message")
}

func runInit() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	if err := preflight.CheckGitRepo(dir); err != nil {
		return err
	}

	i := initialize.New(dir, nil)
	return i.Run()
}

func runServe() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	const opencodeURL = "http://localhost:4096"

	fmt.Println("Running preflight checks...")
	results := preflight.RunAll(dir, opencodeURL, func(name string, index, total int, status string) {
		desc := preflight.GetCheckDescription(name)
		if status == "running" {
			fmt.Printf("  [%d/%d] → %s — %s\n", index, total, name, desc)
		} else if status == "ok" {
			fmt.Printf("  [%d/%d] ✓ %s\n", index, total, name)
		} else {
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
	var spawnedServer *opencode.Server
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
				if status == "running" {
					fmt.Printf("  [%d/%d] → %s — %s\n", index, total, name, desc)
				} else if status == "ok" {
					fmt.Printf("  [%d/%d] ✓ %s\n", index, total, name)
				} else {
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
		return fmt.Errorf("preflight checks failed")
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
	defer store.Close()

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
		if _, err := gh.EnsureProject("ODA"); err != nil {
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
				if err := store.SaveIssueCache(issue, activeMilestone.Title); err != nil {
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

	// Step 6: Create WebSocket hub
	fmt.Println("Creating WebSocket hub...")
	hub := dashboard.NewHub()
	go hub.Run()
	fmt.Println("  ✓ WebSocket hub started")

	// Step 7 & 8: Create and start SyncService
	fmt.Println("Creating sync service...")
	syncService := dashboard.NewSyncService(gh, store, hub)
	if activeMilestone != nil {
		syncService.SetActiveMilestone(activeMilestone.Title)
	}
	syncService.Start()
	fmt.Println("  ✓ Sync service started (30s interval)")

	// Create LLM router for multi-model configuration
	router := llm.NewRouter(&cfg.LLM)

	s := setup.New(dir, oc, cfg, router)
	if err := s.CheckAndGenerate(); err != nil {
		return fmt.Errorf("setup check failed: %w", err)
	}

	brMgr := git.NewBranchManager(dir)

	processor := worker.NewProcessor(cfg, oc, gh, store, brMgr, router)

	orchestrator := mvp.NewOrchestrator(cfg, gh, oc, brMgr, store, hub, router)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("Starting %d workers...\n", cfg.Workers.Count)
	pool := worker.NewPool(cfg.Workers.Count, &worker.EmptyQueue{}, processor)
	pool.Start(ctx)

	go func() {
		if err := orchestrator.Run(ctx); err != nil && err != context.Canceled {
			fmt.Fprintf(os.Stderr, "orchestrator error: %v\n", err)
		}
	}()

	srv, err := dashboard.NewServer(cfg.Dashboard.Port, store, pool.Workers, gh, orchestrator, oc, cfg.Planning.LLM, hub, syncService)
	if err != nil {
		return fmt.Errorf("creating dashboard server: %w", err)
	}

	fmt.Printf("Dashboard: http://localhost:%d\n", cfg.Dashboard.Port)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	srvErrCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
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

	// Add models from pipeline stages
	for _, stage := range cfg.Pipeline.Stages {
		add(stage.LLM)
	}

	// Add legacy config models for backward compatibility
	add(cfg.Planning.LLM)
	add(cfg.EpicAnalysis.LLM)

	// Add models from new LLM config
	add(cfg.LLM.Development.Strong.ToModelRef())
	add(cfg.LLM.Development.Weak.ToModelRef())
	add(cfg.LLM.Planning.Strong.ToModelRef())
	add(cfg.LLM.Planning.Weak.ToModelRef())
	add(cfg.LLM.Orchestration.Strong.ToModelRef())
	add(cfg.LLM.Orchestration.Weak.ToModelRef())
	add(cfg.LLM.Setup.Strong.ToModelRef())
	add(cfg.LLM.Setup.Weak.ToModelRef())

	return models
}
