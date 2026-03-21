package main

import (
	"fmt"
	"os"

	"github.com/one-dev-army/oda/internal/config"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("config loaded: repo=%s, workers=%d\n", cfg.GitHub.Repo, cfg.Workers.Count)
}
