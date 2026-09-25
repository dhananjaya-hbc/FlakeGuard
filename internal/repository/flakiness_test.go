package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"
	"time"
)

const seededTestName = "TestSubject"

// testRepository connects to the database named by TEST_DATABASE_URL.
//
// It deliberately does not fall back to DATABASE_URL: these tests insert and
// delete rows, and must never run against a database holding real data.
func testRepository(t *testing.T) *Repository {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database tests")
	}

	repo, err := New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(repo.Close)
	return repo
}

// uniqueRepo returns a repo identifier unique to this test. Every query in the
// repository layer is scoped by repo, so this keeps cases isolated from each
// other without truncating shared tables.
func uniqueRepo(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("flakeguard-test/%s/%d", t.Name(), time.Now().UnixNano())
}

type seedRun struct {
	branch string
	status string
}

func branchRuns(branch string, statuses ...string) []seedRun {
	runs := make([]seedRun, len(statuses))
	for i, status := range statuses {
		runs[i] = seedRun{branch: branch, status: status}
	}
	return runs
}

// seed writes runs in the given order, one minute apart, and removes them when
// the test finishes.
func seed(t *testing.T, r *Repository, repo string, runs []seedRun) {
	t.Helper()
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := make([]TestRun, len(runs))
	for i, run := range runs {
		records[i] = TestRun{
			Repo:       repo,
			Branch:     run.branch,
			CommitSHA:  fmt.Sprintf("sha-%d", i),
			TestName:   seededTestName,
			Status:     run.status,
			DurationMS: 1,
			CIRunID:    fmt.Sprintf("ci-%d", i),
			RanAt:      base.Add(time.Duration(i) * time.Minute),
		}
	}

	if err := r.RecordRuns(ctx, records); err != nil {
		t.Fatalf("seeding runs: %v", err)
	}

	t.Cleanup(func() {
		if _, err := r.db.Exec(ctx, `DELETE FROM test_runs WHERE repo = $1`, repo); err != nil {
			t.Errorf("cleaning up test_runs: %v", err)
		}
		if _, err := r.db.Exec(ctx, `DELETE FROM test_flakiness WHERE repo = $1`, repo); err != nil {
			t.Errorf("cleaning up test_flakiness: %v", err)
		}
	})
}

func TestRefreshFlakinessScores(t *testing.T) {
	const (
		pass = StatusPassed
		fail = StatusFailed
		skip = StatusSkipped
	)

	cases := []struct {
		name       string
		runs       []seedRun
		wantTotal  int
		wantFailed int
		wantFlips  int
		wantScore  float64
	}{
		{
			name:      "alternating results score as fully flaky",
			runs:      branchRuns("main", pass, fail, pass, fail, pass),
			wantTotal: 5, wantFailed: 2, wantFlips: 4, wantScore: 1,
		},
		{
			name:      "always passing is stable",
			runs:      branchRuns("main", pass, pass, pass, pass),
			wantTotal: 4, wantFailed: 0, wantFlips: 0, wantScore: 0,
		},
		{
			// The claim the whole product rests on: a test that always fails is
			// broken, not flaky. Its failure rate is 100% but it never flips.
			name:      "always failing is broken, not flaky",
			runs:      branchRuns("main", fail, fail, fail, fail),
			wantTotal: 4, wantFailed: 4, wantFlips: 0, wantScore: 0,
		},
		{
			// One run means zero consecutive pairs; GREATEST(total_runs-1, 1)
			// is what keeps this from dividing by zero.
			name:      "single run does not divide by zero",
			runs:      branchRuns("main", pass),
			wantTotal: 1, wantFailed: 0, wantFlips: 0, wantScore: 0,
		},
		{
			name:      "only changes between consecutive runs count",
			runs:      branchRuns("main", pass, pass, fail, pass, fail),
			wantTotal: 5, wantFailed: 2, wantFlips: 3, wantScore: 0.75,
		},
		{
			// Flips are counted within a branch, but the denominator is global
			// (total_runs - 1). The pair straddling the branch boundary is never
			// compared, so a test that flips on every comparable pair still
			// scores below 1.0 once its history spans more than one branch.
			//
			// The branches are arranged so that main ends on pass and feature
			// begins on fail: if the query stopped partitioning by branch, that
			// boundary would register as a fifth flip and this case would fail.
			name: "flips do not cross branch boundaries",
			runs: append(
				branchRuns("main", pass, fail, pass),
				branchRuns("feature", fail, pass, fail)...,
			),
			wantTotal: 6, wantFailed: 3, wantFlips: 4, wantScore: 0.8,
		},
		{
			// Deliberate: any status change counts, including to and from
			// skipped. Asserted here so changing that decision has to be explicit.
			name:      "skipped transitions count as flips",
			runs:      branchRuns("main", pass, skip, fail),
			wantTotal: 3, wantFailed: 1, wantFlips: 2, wantScore: 1,
		},
	}

	r := testRepository(t)
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := uniqueRepo(t)
			seed(t, r, repo, tc.runs)

			if err := r.RefreshFlakinessScores(ctx, repo); err != nil {
				t.Fatalf("RefreshFlakinessScores: %v", err)
			}

			got, err := r.Flakiness(ctx, repo, seededTestName)
			if err != nil {
				t.Fatalf("Flakiness: %v", err)
			}

			if got.TotalRuns != tc.wantTotal {
				t.Errorf("total_runs = %d, want %d", got.TotalRuns, tc.wantTotal)
			}
			if got.FailedRuns != tc.wantFailed {
				t.Errorf("failed_runs = %d, want %d", got.FailedRuns, tc.wantFailed)
			}
			if got.FlipCount != tc.wantFlips {
				t.Errorf("flip_count = %d, want %d", got.FlipCount, tc.wantFlips)
			}
			if math.Abs(got.FlakinessScore-tc.wantScore) > 1e-9 {
				t.Errorf("flakiness_score = %v, want %v", got.FlakinessScore, tc.wantScore)
			}
		})
	}
}

// Refreshing one repo must not read from or write to another's history.
func TestRefreshFlakinessScoresIsolatesRepos(t *testing.T) {
	r := testRepository(t)
	ctx := context.Background()

	target := uniqueRepo(t) + "/target"
	other := uniqueRepo(t) + "/other"

	seed(t, r, target, branchRuns("main", StatusPassed, StatusPassed))
	seed(t, r, other, branchRuns("main", StatusPassed, StatusFailed, StatusPassed))

	if err := r.RefreshFlakinessScores(ctx, target); err != nil {
		t.Fatalf("RefreshFlakinessScores: %v", err)
	}

	if _, err := r.Flakiness(ctx, other, seededTestName); !errors.Is(err, ErrNotFound) {
		t.Errorf("refreshing %q produced a score row for %q: err = %v, want ErrNotFound", target, other, err)
	}
}

// A second refresh must update the existing row in place rather than failing on
// the (repo, test_name) primary key or leaving stale counts behind.
func TestRefreshFlakinessScoresUpsertsOnRepeat(t *testing.T) {
	r := testRepository(t)
	ctx := context.Background()
	repo := uniqueRepo(t)

	seed(t, r, repo, branchRuns("main", StatusPassed, StatusPassed, StatusPassed))

	if err := r.RefreshFlakinessScores(ctx, repo); err != nil {
		t.Fatalf("first RefreshFlakinessScores: %v", err)
	}

	first, err := r.Flakiness(ctx, repo, seededTestName)
	if err != nil {
		t.Fatalf("Flakiness after first refresh: %v", err)
	}
	if first.TotalRuns != 3 || first.FlipCount != 0 {
		t.Fatalf("after first refresh: total_runs = %d, flip_count = %d; want 3 and 0", first.TotalRuns, first.FlipCount)
	}

	// A later CI run reports a failure, which should register as one flip.
	if err := r.RecordRun(ctx, TestRun{
		Repo:       repo,
		Branch:     "main",
		CommitSHA:  "sha-later",
		TestName:   seededTestName,
		Status:     StatusFailed,
		DurationMS: 1,
		CIRunID:    "ci-later",
		RanAt:      time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	if err := r.RefreshFlakinessScores(ctx, repo); err != nil {
		t.Fatalf("second RefreshFlakinessScores: %v", err)
	}

	second, err := r.Flakiness(ctx, repo, seededTestName)
	if err != nil {
		t.Fatalf("Flakiness after second refresh: %v", err)
	}
	if second.TotalRuns != 4 {
		t.Errorf("total_runs = %d, want 4", second.TotalRuns)
	}
	if second.FailedRuns != 1 {
		t.Errorf("failed_runs = %d, want 1", second.FailedRuns)
	}
	if second.FlipCount != 1 {
		t.Errorf("flip_count = %d, want 1", second.FlipCount)
	}
	if want := 1.0 / 3.0; math.Abs(second.FlakinessScore-want) > 1e-9 {
		t.Errorf("flakiness_score = %v, want %v", second.FlakinessScore, want)
	}
	if !second.LastUpdated.After(first.LastUpdated) {
		t.Errorf("last_updated did not advance: first = %v, second = %v", first.LastUpdated, second.LastUpdated)
	}
}
