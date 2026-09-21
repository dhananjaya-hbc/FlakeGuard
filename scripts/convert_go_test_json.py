#!/usr/bin/env python3
"""Convert `go test -json` output into the FlakeGuard /ingest report format."""

import argparse
import json
import os
import sys


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "input",
        nargs="?",
        default="-",
        help="path to go test -json output, or '-' for stdin (default: stdin)",
    )
    parser.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY"))
    parser.add_argument("--branch", default=os.environ.get("GITHUB_REF_NAME"))
    parser.add_argument("--commit-sha", default=os.environ.get("GITHUB_SHA"))
    parser.add_argument("--ci-run-id", default=os.environ.get("GITHUB_RUN_ID"))
    args = parser.parse_args()

    missing = [
        name
        for name, value in [
            ("--repo", args.repo),
            ("--branch", args.branch),
            ("--commit-sha", args.commit_sha),
            ("--ci-run-id", args.ci_run_id),
        ]
        if not value
    ]
    if missing:
        parser.error(
            f"missing required value(s): {', '.join(missing)} "
            "(pass as a flag, or run in GitHub Actions where they're read from the environment)"
        )
    return args


STATUS_BY_ACTION = {"pass": "passed", "fail": "failed", "skip": "skipped"}


def convert(lines):
    tests = []
    for line in lines:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue

        test_name = event.get("Test")
        action = event.get("Action")
        if not test_name or action not in STATUS_BY_ACTION:
            continue

        tests.append(
            {
                "test_name": test_name,
                "status": STATUS_BY_ACTION[action],
                "duration_ms": round(event.get("Elapsed", 0) * 1000),
                "ran_at": event["Time"],
            }
        )
    return tests


def main():
    args = parse_args()

    input_stream = sys.stdin if args.input == "-" else open(args.input, "r")
    try:
        tests = convert(input_stream)
    finally:
        if input_stream is not sys.stdin:
            input_stream.close()

    if not tests:
        print("no test pass/fail/skip events found in input", file=sys.stderr)
        sys.exit(1)

    report = {
        "repo": args.repo,
        "branch": args.branch,
        "commit_sha": args.commit_sha,
        "ci_run_id": args.ci_run_id,
        "tests": tests,
    }
    json.dump(report, sys.stdout)


if __name__ == "__main__":
    main()
