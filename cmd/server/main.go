package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dhananjaya/flakeguard/internal/ingest"
	"github.com/dhananjaya/flakeguard/internal/mcp"
	"github.com/dhananjaya/flakeguard/internal/repository"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: server <http|mcp>")
		os.Exit(1)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	ctx := context.Background()
	repo, err := repository.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer repo.Close()

	switch os.Args[1] {
	case "mcp":
		runMCP(ctx, repo)
	case "http":
		runHTTP(ctx, repo)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q: expected \"http\" or \"mcp\"\n", os.Args[1])
		os.Exit(1)
	}
}

func runMCP(ctx context.Context, repo *repository.Repository) {
	server := mcp.NewServer("flakeguard", "0.1.0")
	mcp.RegisterTools(server, repo)

	if err := server.Run(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("mcp server error: %v", err)
	}
}

func runHTTP(ctx context.Context, repo *repository.Repository) {
	mux := http.NewServeMux()
	mux.Handle("/ingest", ingest.NewHandler(repo))

	go refreshFlakinessLoop(ctx, repo)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http server error: %v", err)
	}
}

func refreshFlakinessLoop(ctx context.Context, repo *repository.Repository) {
	interval := 2 * time.Minute
	if v := os.Getenv("FLAKINESS_REFRESH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		} else {
			log.Printf("invalid FLAKINESS_REFRESH_INTERVAL %q, using default %s", v, interval)
		}
	}

	refreshAllRepos(ctx, repo)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		refreshAllRepos(ctx, repo)
	}
}

func refreshAllRepos(ctx context.Context, repo *repository.Repository) {
	repos, err := repo.DistinctRepos(ctx)
	if err != nil {
		log.Printf("listing repos for flakiness refresh: %v", err)
		return
	}
	for _, r := range repos {
		if err := repo.RefreshFlakinessScores(ctx, r); err != nil {
			log.Printf("refreshing flakiness scores for %s: %v", r, err)
		}
	}
	log.Printf("refreshed flakiness scores for %d repos", len(repos))
}
