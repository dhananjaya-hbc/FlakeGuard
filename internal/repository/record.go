package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

func (r *Repository) RecordRuns(ctx context.Context, runs []TestRun) error {
	if len(runs) == 0 {
		return nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const query = `
		INSERT INTO test_runs (repo, branch, commit_sha, test_name, status, duration_ms, ci_run_id, ran_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	batch := &pgx.Batch{}
	for _, run := range runs {
		batch.Queue(query, run.Repo, run.Branch, run.CommitSHA, run.TestName, run.Status, run.DurationMS, run.CIRunID, run.RanAt)
	}

	br := tx.SendBatch(ctx, batch)
	for range runs {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("inserting test run: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("closing batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}
