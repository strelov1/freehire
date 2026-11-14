"""Fail a pull request whose hand-written change is too big to review."""

import argparse
import fnmatch
import os
import subprocess

MAX_FILES = 30
MAX_LINES = 1500
OVERRIDE_LABEL = "large-pr"
# Tools rewrite these; nobody reviews them line by line.
GENERATED = (
    "*.sql.go",
    "internal/platform/db/batch.go",
    "internal/platform/db/copyfrom.go",
    "internal/platform/db/db.go",
    "internal/platform/db/models.go",
    "internal/platform/db/querier.go",
    "internal/api/apigen/*",
    "web/src/lib/generated/*",
    "go.sum",
    "*pnpm-lock.yaml",
    "*package-lock.json",
)


def parse_numstat(text: str) -> tuple[int, int]:
    """Count files and changed lines in `git diff --numstat -z --no-renames` output."""
    files = lines = 0
    for record in filter(None, text.split("\0")):
        added, deleted, path = record.split("\t", 2)
        if any(fnmatch.fnmatch(path, pattern) for pattern in GENERATED):
            continue
        files += 1
        # Binary files report "-": they count as a file but not as lines.
        lines += sum(int(n) for n in (added, deleted) if n != "-")
    return files, lines


def problem(files: int, lines: int) -> str | None:
    if files <= MAX_FILES and lines <= MAX_LINES:
        return None
    return (
        f"this PR changes {files} files and {lines} lines without generated files; "
        f"the limit is {MAX_FILES} files and {MAX_LINES} lines"
    )


def check(base: str, head: str) -> str | None:
    diff = subprocess.run(
        ["git", "diff", "--numstat", "-z", "--no-renames", f"{base}...{head}"],
        capture_output=True,
        text=True,
        check=True,
    )
    return problem(*parse_numstat(diff.stdout))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True)
    parser.add_argument("--head", required=True)
    args = parser.parse_args()
    reason = check(args.base, args.head)
    if reason is None:
        print("PR size is within the limit.")
    elif os.environ.get("LARGE_PR") == "true":
        print(f"Allowed by the {OVERRIDE_LABEL} label: {reason}.")
    else:
        print(
            f"::error::Split this PR: {reason}. "
            f"If it cannot be split (a mechanical rename, a vendored update), "
            f"add the {OVERRIDE_LABEL} label."
        )
        raise SystemExit(1)
