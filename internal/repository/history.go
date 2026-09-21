package repository

import (
	"context"
	"fmt"
)

func (r *Repository) History(ctx context.Context, repo, testName string, limit int) ([]HistoryEntry, error) {
	const query = `
		SELECT branch, commit_sha, status, duration_ms, ci_run_id, ran_at
		FROM test_runs
		WHERE repo = $1 AND test_name = $2
		ORDER BY ran_at DESC
		LIMIT $3
	`
	rows, err := r.db.Query(ctx, query, repo, testName, limit)
	if err != nil {
		return nil, fmt.Errorf("querying test history: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.Branch, &e.CommitSHA, &e.Status, &e.DurationMS, &e.CIRunID, &e.RanAt); err != nil {
			return nil, fmt.Errorf("scanning test history row: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating test history rows: %w", err)
	}
	return entries, nil
}
