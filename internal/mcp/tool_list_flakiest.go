package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dhananjaya/flakeguard/internal/repository"
)

func listFlakiestTestsTool() Tool {
	return Tool{
		Name: "list_flakiest_tests",
		Description: `Get a ranked list of the flakiest tests in a repo, highest flakiness score first. Call this for broad questions like "which tests are unreliable in this repo" or when deciding which flaky tests are worth fixing first — not for questions about one specific test (use check_test_flakiness for that instead).`,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"repo":  {Type: "string", Description: "The repository name exactly as recorded in CI, e.g. \"myorg/myrepo\"."},
				"limit": {Type: "integer", Description: "Maximum number of tests to return, ranked by flakiness score descending. Defaults to 10, capped at 50."},
			},
			Required: []string{"repo"},
		},
	}
}

type listFlakiestTestsArgs struct {
	Repo  string `json:"repo"`
	Limit int    `json:"limit"`
}

func listFlakiestTestsHandler(repo *repository.Repository) ToolHandler {
	return func(ctx context.Context, arguments json.RawMessage) (*ToolCallResult, error) {
		var args listFlakiestTestsArgs
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		if args.Repo == "" {
			return nil, fmt.Errorf("repo is required")
		}

		limit := args.Limit
		if limit <= 0 {
			limit = 10
		}
		if limit > 50 {
			limit = 50
		}

		results, err := repo.FlakiestTests(ctx, args.Repo, limit)
		if err != nil {
			return nil, fmt.Errorf("looking up flakiest tests: %w", err)
		}
		if len(results) == 0 {
			return textResult(fmt.Sprintf("No flakiness data found for repo %q.", args.Repo)), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Flakiest tests in %s, ranked by score:\n", args.Repo)
		for i, f := range results {
			fmt.Fprintf(&b, "%d. %s — score=%.2f (%s) | %d/%d failed | %d flips\n",
				i+1, f.TestName, f.FlakinessScore, flakinessVerdict(f.FlakinessScore),
				f.FailedRuns, f.TotalRuns, f.FlipCount)
		}

		return textResult(b.String()), nil
	}
}
