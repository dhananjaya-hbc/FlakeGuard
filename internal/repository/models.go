package repository

import "time"

const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

type TestRun struct {
	Repo       string
	Branch     string
	CommitSHA  string
	TestName   string
	Status     string
	DurationMS int
	CIRunID    string
	RanAt      time.Time
}

type HistoryEntry struct {
	Branch     string
	CommitSHA  string
	Status     string
	DurationMS int
	CIRunID    string
	RanAt      time.Time
}

type FlakinessResult struct {
	Repo           string
	TestName       string
	TotalRuns      int
	FailedRuns     int
	FlipCount      int
	FlakinessScore float64
	LastUpdated    time.Time
}
