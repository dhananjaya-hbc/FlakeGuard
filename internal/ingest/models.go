package ingest

import (
	"fmt"
	"time"

	"github.com/dhananjaya-hbc/FlakeGuard/internal/repository"
)

type Report struct {
	Repo      string       `json:"repo"`
	Branch    string       `json:"branch"`
	CommitSHA string       `json:"commit_sha"`
	CIRunID   string       `json:"ci_run_id"`
	Tests     []TestResult `json:"tests"`
}

type TestResult struct {
	TestName   string    `json:"test_name"`
	Status     string    `json:"status"`
	DurationMS int       `json:"duration_ms"`
	RanAt      time.Time `json:"ran_at"`
}

func (r Report) Validate() error {
	if r.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if r.Branch == "" {
		return fmt.Errorf("branch is required")
	}
	if r.CommitSHA == "" {
		return fmt.Errorf("commit_sha is required")
	}
	if r.CIRunID == "" {
		return fmt.Errorf("ci_run_id is required")
	}
	if len(r.Tests) == 0 {
		return fmt.Errorf("tests must not be empty")
	}
	for i, t := range r.Tests {
		if t.TestName == "" {
			return fmt.Errorf("tests[%d]: test_name is required", i)
		}
		switch t.Status {
		case repository.StatusPassed, repository.StatusFailed, repository.StatusSkipped:
		default:
			return fmt.Errorf("tests[%d]: invalid status %q", i, t.Status)
		}
		if t.RanAt.IsZero() {
			return fmt.Errorf("tests[%d]: ran_at is required", i)
		}
	}
	return nil
}
