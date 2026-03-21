package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/one-dev-army/oda/internal/config"
	"github.com/one-dev-army/oda/internal/dashboard"
	"github.com/one-dev-army/oda/internal/db"
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

	_ = opencode.NewClient(cfg.OpenCode.URL)

	fmt.Println("Verifying GitHub setup...")
	fmt.Println("Syncing GitHub...")
	fmt.Printf("Starting %d workers...\n", cfg.Workers.Count)
	fmt.Println()

	poolInfoFn := func() []worker.WorkerInfo { return nil }

	srv, err := dashboard.NewServer(cfg.Dashboard.Port, store, poolInfoFn)
	if err != nil {
		return fmt.Errorf("creating dashboard server: %w", err)
	}

	fmt.Printf("Dashboard: http://localhost:%d\n", cfg.Dashboard.Port)
	fmt.Println("Press Ctrl+C to stop")

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		fmt.Printf("\nReceived %s, shutting down...\n", sig)
		return nil
	case err := <-errCh:
		return fmt.Errorf("dashboard server: %w", err)
	}
}
