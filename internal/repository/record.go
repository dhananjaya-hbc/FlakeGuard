package repository

import (
	"context"
	"fmt"
)

func (r *Repository) RecordRun(ctx context.Context, run TestRun) error {
	const query = `
		INSERT INTO test_runs (repo, branch, commit_sha, test_name, status, duration_ms, ci_run_id, ran_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.Exec(ctx, query,
		run.Repo, run.Branch, run.CommitSHA, run.TestName, run.Status, run.DurationMS, run.CIRunID, run.RanAt,
	)
	if err != nil {
		return fmt.Errorf("inserting test run: %w", err)
	}
	return nil
}
