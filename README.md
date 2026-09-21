# FlakeGuard

Flaky test detection for CI pipelines, exposed to AI coding assistants over MCP — so when a developer's test fails, their assistant can tell them whether it's a real bug or a known-flaky test, right inside the editor.

## The problem

Tests that fail intermittently — not because of a real bug, but timing issues, race conditions, or flaky external dependencies — waste huge amounts of developer time. A red CI check could mean "you broke something" or "this test is just unreliable," and there's usually no easy way to tell which. FlakeGuard tracks test history over time and answers that question automatically, using the AI coding assistant the developer already has open.

## How it works

1. A developer pushes code, and GitHub Actions runs the test suite.
2. A CI step converts `go test -json` output into a small JSON report and POSTs it to FlakeGuard's `/ingest` endpoint.
3. FlakeGuard writes every individual test result — repo, branch, commit, test name, status, duration, timestamp — into Postgres.
4. A background job periodically recomputes a **flakiness score** per test: a test "flips" whenever consecutive runs on the same branch have different results (pass then fail, or vice versa). `flakiness_score = flip_count / (total_runs - 1)` — a score near 0 means stable, a score near 1 means the result is essentially noise.
5. An MCP server exposes that data to AI coding assistants as three tools: `check_test_flakiness`, `get_test_history`, and `list_flakiest_tests`.
6. A developer asks "why did this test fail?" — their assistant calls `check_test_flakiness` and gives a grounded answer instead of guessing.

## Quick start (Docker Compose)

This brings up Postgres (with the schema auto-applied) and the ingestion API together:

```bash
cp .env.example .env   # fill in INGEST_API_KEY with a secret of your choosing
docker compose up -d --build
```

The API listens on `http://localhost:8080`. Verify it's up:

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <your INGEST_API_KEY>" \
  -d '{
    "repo": "myorg/myrepo",
    "branch": "main",
    "commit_sha": "abc123",
    "ci_run_id": "1",
    "tests": [
      {"test_name": "TestExample", "status": "passed", "duration_ms": 42, "ran_at": "2026-01-01T00:00:00Z"}
    ]
  }'
```

You should get back `{"recorded":1}` with a `201` status.

## Local development (without Docker)

You'll need Go 1.22+ and a local Postgres.

```bash
createdb flakeguard_dev
psql -d flakeguard_dev -f migrations/001_init.sql

# .env
DATABASE_URL=postgres://<user>@localhost:5432/flakeguard_dev?sslmode=disable
INGEST_API_KEY=local-dev-secret

go build -o flakeguard ./cmd/server

# terminal 1
set -a; source .env; set +a
./flakeguard http

# terminal 2 — try the MCP server directly over stdio
set -a; source .env; set +a
./flakeguard mcp
```

In `mcp` mode, the binary speaks newline-delimited JSON-RPC 2.0 over stdin/stdout — it's meant to be launched as a subprocess by an MCP client (see below), not used interactively, though you can feed it raw JSON-RPC lines for testing.

## Connecting to Claude Code

Add a `.mcp.json` file at your project root (or wherever you want FlakeGuard's data available from):

```json
{
  "mcpServers": {
    "flakeguard": {
      "type": "stdio",
      "command": "/absolute/path/to/flakeguard",
      "args": ["mcp"],
      "env": {
        "DATABASE_URL": "postgres://user@localhost:5432/flakeguard_dev?sslmode=disable"
      }
    }
  }
}
```

Or register it via the CLI:

```bash
claude mcp add --transport stdio flakeguard --scope project \
  --env DATABASE_URL="postgres://user@localhost:5432/flakeguard_dev?sslmode=disable" \
  -- /absolute/path/to/flakeguard mcp
```

Restart Claude Code (or run `/mcp` to check connection status) and the three tools below become available to the assistant automatically — no further setup needed on the assistant side.

## Worked example

Say `TestFoo` in `myorg/myrepo` just failed in CI. A developer asks Claude Code "did I break something, or is this test just flaky?" Claude Code calls `check_test_flakiness`:

**Request** (what the assistant sends, over stdio):
```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"check_test_flakiness","arguments":{"repo":"myorg/myrepo","test_name":"TestFoo"}}}
```

**Response** (what FlakeGuard sends back):
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Test: TestFoo\nRepo: myorg/myrepo\nFlakiness score: 0.75\nVerdict: highly flaky — failures here are likely noise, not a real bug\nTotal runs: 5\nFailed runs: 2\nFlip count: 3\nLast updated: 2026-01-01T12:00:00Z"
      }
    ]
  }
}
```

Claude Code then tells the developer: *"This test has a flakiness score of 0.75 — it's flipped between pass and fail 3 times over its last 5 runs. This failure is very likely noise, not something you broke. Want me to check `get_test_history` for the exact timeline, or look at other flaky tests in this repo?"* — instead of assuming the developer's change caused the failure.

## The three MCP tools

| Tool | When it's called | Returns |
|---|---|---|
| `check_test_flakiness(repo, test_name)` | A specific test failed and you need a verdict | Flakiness score, plain-language verdict, run/failure/flip counts |
| `get_test_history(repo, test_name, limit=20, max=100)` | You want the raw pass/fail/skip timeline for one test | Recent runs, most recent first |
| `list_flakiest_tests(repo, limit=10, max=50)` | Broad triage — "which tests are unreliable here?" | Ranked list of tests by flakiness score |

## Ingestion API

`POST /ingest`, header `X-API-Key: <INGEST_API_KEY>`, body:

```json
{
  "repo": "myorg/myrepo",
  "branch": "main",
  "commit_sha": "<sha>",
  "ci_run_id": "<ci run identifier>",
  "tests": [
    {"test_name": "TestFoo", "status": "passed", "duration_ms": 120, "ran_at": "2026-01-01T00:00:00Z"}
  ]
}
```

`status` must be one of `passed`, `failed`, `skipped`. All fields are required. Responds `201 {"recorded": N}` on success, `400` on validation errors, `401` on a missing/incorrect API key.

`scripts/convert_go_test_json.py` builds this report from `go test -json` output:

```bash
go test -json ./... | python3 scripts/convert_go_test_json.py \
  --repo myorg/myrepo --branch main --commit-sha "$SHA" --ci-run-id "$RUN_ID" \
  > report.json
```

When run inside GitHub Actions, `--repo`/`--branch`/`--commit-sha`/`--ci-run-id` are auto-detected from `GITHUB_REPOSITORY`/`GITHUB_REF_NAME`/`GITHUB_SHA`/`GITHUB_RUN_ID` if not passed explicitly. See `.github/workflows/example-usage.yml` for the full loop — it needs two repo secrets: `FLAKEGUARD_INGEST_URL` (your deployed API's base URL) and `FLAKEGUARD_API_KEY`.

## Environment variables

| Variable | Required in | Purpose |
|---|---|---|
| `DATABASE_URL` | both modes | Postgres connection string |
| `INGEST_API_KEY` | `http` mode | Shared secret required on all `/ingest` requests (`X-API-Key` header) |
| `HTTP_ADDR` | `http` mode | Listen address, defaults to `:8080` |
| `FLAKINESS_REFRESH_INTERVAL` | `http` mode | How often to recompute flakiness scores, defaults to `2m` (Go duration syntax) |

## Project structure

```
cmd/server/main.go          # entrypoint, "http" or "mcp" mode via arg
internal/
  mcp/                      # protocol types, tool definitions, stdio transport loop
  ingest/                   # HTTP handler + auth for /ingest
  repository/               # all Postgres queries — nothing else writes SQL
migrations/001_init.sql
scripts/convert_go_test_json.py
.github/workflows/example-usage.yml
Dockerfile
docker-compose.yml
```
