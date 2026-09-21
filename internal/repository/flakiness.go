package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("not found")

func (r *Repository) Flakiness(ctx context.Context, repo, testName string) (*FlakinessResult, error) {
	const query = `
		SELECT repo, test_name, total_runs, failed_runs, flip_count, flakiness_score, last_updated
		FROM test_flakiness
		WHERE repo = $1 AND test_name = $2
	`
	var f FlakinessResult
	err := r.db.QueryRow(ctx, query, repo, testName).Scan(
		&f.Repo, &f.TestName, &f.TotalRuns, &f.FailedRuns, &f.FlipCount, &f.FlakinessScore, &f.LastUpdated,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying flakiness: %w", err)
	}
	return &f, nil
}

func (r *Repository) FlakiestTests(ctx context.Context, repo string, limit int) ([]FlakinessResult, error) {
	const query = `
		SELECT repo, test_name, total_runs, failed_runs, flip_count, flakiness_score, last_updated
		FROM test_flakiness
		WHERE repo = $1
		ORDER BY flakiness_score DESC
		LIMIT $2
	`
	rows, err := r.db.Query(ctx, query, repo, limit)
	if err != nil {
		return nil, fmt.Errorf("querying flakiest tests: %w", err)
	}
	defer rows.Close()

	var results []FlakinessResult
	for rows.Next() {
		var f FlakinessResult
		if err := rows.Scan(&f.Repo, &f.TestName, &f.TotalRuns, &f.FailedRuns, &f.FlipCount, &f.FlakinessScore, &f.LastUpdated); err != nil {
			return nil, fmt.Errorf("scanning flakiest test row: %w", err)
		}
		results = append(results, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating flakiest test rows: %w", err)
	}
	return results, nil
}

func (r *Repository) RefreshFlakinessScores(ctx context.Context, repo string) error {
	const query = `
		WITH flips AS (
			SELECT
				test_name,
				status,
				LAG(status) OVER (PARTITION BY test_name, branch ORDER BY ran_at) AS prev_status
			FROM test_runs
			WHERE repo = $1
		),
		agg AS (
			SELECT
				test_name,
				COUNT(*) AS total_runs,
				COUNT(*) FILTER (WHERE status = 'failed') AS failed_runs,
				COUNT(*) FILTER (WHERE prev_status IS NOT NULL AND status <> prev_status) AS flip_count
			FROM flips
			GROUP BY test_name
		)
		INSERT INTO test_flakiness (repo, test_name, total_runs, failed_runs, flip_count, flakiness_score, last_updated)
		SELECT
			$1,
			test_name,
			total_runs,
			failed_runs,
			flip_count,
			flip_count::double precision / GREATEST(total_runs - 1, 1),
			now()
		FROM agg
		ON CONFLICT (repo, test_name) DO UPDATE SET
			total_runs = EXCLUDED.total_runs,
			failed_runs = EXCLUDED.failed_runs,
			flip_count = EXCLUDED.flip_count,
			flakiness_score = EXCLUDED.flakiness_score,
			last_updated = EXCLUDED.last_updated
	`
	_, err := r.db.Exec(ctx, query, repo)
	if err != nil {
		return fmt.Errorf("refreshing flakiness scores: %w", err)
	}
	return nil
}
