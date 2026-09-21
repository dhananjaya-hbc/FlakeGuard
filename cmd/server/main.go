package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

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
		runHTTP(repo)
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

func runHTTP(repo *repository.Repository) {
	mux := http.NewServeMux()
	mux.Handle("/ingest", ingest.NewHandler(repo))

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http server error: %v", err)
	}
}
