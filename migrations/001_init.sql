CREATE TABLE test_runs (
    id          BIGSERIAL PRIMARY KEY,
    repo        TEXT NOT NULL,
    branch      TEXT NOT NULL,
    commit_sha  TEXT NOT NULL,
    test_name   TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('passed', 'failed', 'skipped')),
    duration_ms INTEGER NOT NULL,
    ci_run_id   TEXT NOT NULL,
    ran_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_test_runs_history
    ON test_runs (repo, test_name, ran_at DESC);

CREATE INDEX idx_test_runs_flakiness_scan
    ON test_runs (repo, test_name, branch, ran_at);

CREATE TABLE test_flakiness (
    repo             TEXT NOT NULL,
    test_name        TEXT NOT NULL,
    total_runs       INTEGER NOT NULL,
    failed_runs      INTEGER NOT NULL,
    flip_count       INTEGER NOT NULL,
    flakiness_score  DOUBLE PRECISION NOT NULL,
    last_updated     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (repo, test_name)
);

CREATE INDEX idx_test_flakiness_ranked
    ON test_flakiness (repo, flakiness_score DESC);
