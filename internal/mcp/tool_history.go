package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dhananjaya/flakeguard/internal/repository"
)

func getTestHistoryTool() Tool {
	return Tool{
		Name: "get_test_history",
		Description: `Get the recent pass/fail/skip history for one specific test, most recent run first. Call this when you want to see the raw timeline behind a test's behavior — for example to visually confirm an alternating pass/fail pattern, check how recently a test last failed, or see which branch/commit a failure happened on. For a summary score instead of raw history, use check_test_flakiness.`,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"repo":      {Type: "string", Description: "The repository name exactly as recorded in CI, e.g. \"myorg/myrepo\"."},
				"test_name": {Type: "string", Description: "The exact test name/identifier as reported by the test runner."},
				"limit":     {Type: "integer", Description: "Maximum number of recent runs to return, most recent first. Defaults to 20, capped at 100."},
			},
			Required: []string{"repo", "test_name"},
		},
	}
}

type getTestHistoryArgs struct {
	Repo     string `json:"repo"`
	TestName string `json:"test_name"`
	Limit    int    `json:"limit"`
}

func getTestHistoryHandler(repo *repository.Repository) ToolHandler {
	return func(ctx context.Context, arguments json.RawMessage) (*ToolCallResult, error) {
		var args getTestHistoryArgs
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		if args.Repo == "" || args.TestName == "" {
			return nil, fmt.Errorf("both repo and test_name are required")
		}

		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		history, err := repo.History(ctx, args.Repo, args.TestName, limit)
		if err != nil {
			return nil, fmt.Errorf("looking up test history: %w", err)
		}
		if len(history) == 0 {
			return textResult(fmt.Sprintf("No run history found for test %q in repo %q.", args.TestName, args.Repo)), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Recent history for %s (repo: %s), most recent first:\n", args.TestName, args.Repo)
		for _, h := range history {
			fmt.Fprintf(&b, "- %s | %s | branch=%s | commit=%s | duration=%dms | ci_run=%s\n",
				h.RanAt.Format(time.RFC3339), h.Status, h.Branch, h.CommitSHA, h.DurationMS, h.CIRunID)
		}

		return textResult(b.String()), nil
	}
}
