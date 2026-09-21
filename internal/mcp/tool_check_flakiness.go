package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dhananjaya/flakeguard/internal/repository"
)

func checkTestFlakinessTool() Tool {
	return Tool{
		Name: "check_test_flakiness",
		Description: `Look up whether a specific test is known to be flaky. Call this whenever a test has failed in CI and you need to determine if the failure indicates a real bug in the code, or is a known-unreliable test that fails intermittently for unrelated reasons (timing, race conditions, flaky external dependencies). Given an exact repo name and test name, returns a flakiness score from 0.0 (never flips between pass/fail) to 1.0 (flips almost every run), a plain-language verdict, and the underlying run/failure/flip counts. Always call this before telling a developer that a failing test means they broke something — a high flakiness score means the failure is likely noise, not their fault.`,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"repo":      {Type: "string", Description: "The repository name exactly as recorded in CI, e.g. \"myorg/myrepo\"."},
				"test_name": {Type: "string", Description: "The exact test name/identifier as reported by the test runner."},
			},
			Required: []string{"repo", "test_name"},
		},
	}
}

type checkTestFlakinessArgs struct {
	Repo     string `json:"repo"`
	TestName string `json:"test_name"`
}

func checkTestFlakinessHandler(repo *repository.Repository) ToolHandler {
	return func(ctx context.Context, arguments json.RawMessage) (*ToolCallResult, error) {
		var args checkTestFlakinessArgs
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		if args.Repo == "" || args.TestName == "" {
			return nil, fmt.Errorf("both repo and test_name are required")
		}

		result, err := repo.Flakiness(ctx, args.Repo, args.TestName)
		if errors.Is(err, repository.ErrNotFound) {
			return textResult(fmt.Sprintf(
				"No flakiness data found for test %q in repo %q. Either it has never run, or fewer than 2 runs have been recorded yet, so a score hasn't been computed. Treat any failure as a real bug until more history accumulates.",
				args.TestName, args.Repo,
			)), nil
		}
		if err != nil {
			return nil, fmt.Errorf("looking up flakiness: %w", err)
		}

		return textResult(fmt.Sprintf(
			"Test: %s\nRepo: %s\nFlakiness score: %.2f\nVerdict: %s\nTotal runs: %d\nFailed runs: %d\nFlip count: %d\nLast updated: %s",
			result.TestName, result.Repo, result.FlakinessScore, flakinessVerdict(result.FlakinessScore),
			result.TotalRuns, result.FailedRuns, result.FlipCount,
			result.LastUpdated.Format(time.RFC3339),
		)), nil
	}
}
