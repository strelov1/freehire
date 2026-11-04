"""Select expensive CI work while keeping cross-file Go unit guards on every run."""

import argparse
from dataclasses import dataclass
import json
import os
from pathlib import Path
import re
import subprocess
import sys


BACKEND_TASKS = (
    "checks",
    "unit",
    "integration-handler",
    "integration-db",
    "integration-cmd",
    "integration-engage-identity",
    "integration-rest",
)


@dataclass(frozen=True)
class Scope:
    backend: bool
    web: bool

    @property
    def backend_tasks(self):
        return BACKEND_TASKS if self.backend else ("unit",)


def classify(paths):
    if not paths:
        return Scope(backend=True, web=True)

    backend = False
    web = False
    for path in paths:
        # Go files must not escape tagged compilation by being added elsewhere.
        backend = backend or path.endswith(".go")
        if path.startswith(("web/", "design-system/")):
            web = True
        elif path.startswith(("internal/", "cmd/", "migrations/")) or path in (
            "go.mod", "go.sum", ".golangci.yml"
        ):
            backend = True
            # PlanView.test.ts reads the Go feature vocabulary directly.
            web = web or path.startswith("internal/ai/plan/")
        elif path in ("docs/API.md", "docs/API.internal.md"):
            # These are generated and verified by the web job.
            web = True
        elif path.startswith(("docs/", "openspec/", "extension/")):
            # Go unit guards still inspect promo documents and extension fixtures.
            continue
        elif "/" not in path and path.endswith(".md"):
            continue
        else:
            # CI, shared scripts/configuration and unfamiliar inputs run everything.
            return Scope(backend=True, web=True)
    return Scope(backend=backend, web=web)


def read_changes(base, head, repo=Path(".")):
    valid_shas = all(
        re.fullmatch(r"[0-9a-fA-F]{40}|[0-9a-fA-F]{64}", sha or "")
        for sha in (base, head)
    )
    if not valid_shas or set(base) == {"0"}:
        return None
    try:
        result = subprocess.run(
            ["git", "diff", "--no-renames", "--name-only", "-z", base, head, "--"],
            cwd=repo,
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=60,
        )
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        print("Cannot determine changed files; selecting the full CI suite.", file=sys.stderr)
        return None
    return [os.fsdecode(path) for path in result.stdout.split(b"\0") if path]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True)
    parser.add_argument("--head", required=True)
    args = parser.parse_args()
    scope = classify(read_changes(args.base, args.head))
    print(f"backend_tasks={json.dumps(scope.backend_tasks)}")
    print(f"web={str(scope.web).lower()}")


if __name__ == "__main__":
    main()
