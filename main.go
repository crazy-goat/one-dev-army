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

	"github.com/one-dev-army/oda/internal/config"
	"github.com/one-dev-army/oda/internal/dashboard"
	"github.com/one-dev-army/oda/internal/db"
	"github.com/one-dev-army/oda/internal/git"
	"github.com/one-dev-army/oda/internal/github"
	"github.com/one-dev-army/oda/internal/initialize"
	"github.com/one-dev-army/oda/internal/opencode"
	"github.com/one-dev-army/oda/internal/preflight"
	"github.com/one-dev-army/oda/internal/worker"
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

	fmt.Println("Running preflight checks...")
	results := preflight.RunAll(dir, "http://localhost:4096")
	allOK := true
	for _, r := range results {
		if r.OK {
			fmt.Printf("  ✓ %s\n", r.Name)
		} else {
			fmt.Printf("  ✗ %s: %s\n", r.Name, r.Message)
			allOK = false
		}
	}
	if !allOK {
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
	gh := github.NewClient(cfg.GitHub.Repo)

	fmt.Println("Verifying GitHub setup...")
	if err := gh.EnsureLabels(); err != nil {
		return fmt.Errorf("ensuring labels: %w", err)
	}

	worktreesDir := filepath.Join(dir, ".oda", "worktrees")
	if err := os.MkdirAll(worktreesDir, 0o755); err != nil {
		return fmt.Errorf("creating worktrees directory: %w", err)
	}
	wtMgr := git.NewWorktreeManager(dir, worktreesDir)

	processor := worker.NewProcessor(cfg, oc, gh, store, wtMgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("Starting %d workers...\n", cfg.Workers.Count)
	pool := worker.NewPool(cfg.Workers.Count, nil, processor)
	pool.Start(ctx)

	srv, err := dashboard.NewServer(cfg.Dashboard.Port, store, pool.Workers)
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

		go func() {
			<-sigCh
			fmt.Println("\nForce quitting...")
			os.Exit(1)
		}()

		<-workersDone
		fmt.Println("All workers finished.")

	case <-workersDone:
		fmt.Println("All workers finished.")

	case err := <-srvErrCh:
		return fmt.Errorf("dashboard server: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down dashboard: %w", err)
	}

	return nil
}
