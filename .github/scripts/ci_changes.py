"""Select expensive CI work while keeping cross-file Go unit guards on every run."""

import argparse
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

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
    scrapling_fetch: bool = False
    extension: bool = False
    docs: bool = False
    migrations: bool = False
    contracts: bool = False
    sqlc: bool = False
    design_system: bool = False

    @property
    def backend_tasks(self):
        return BACKEND_TASKS if self.backend else ("unit",)


def classify(paths):
    flags = dict.fromkeys(Scope.__dataclass_fields__, False)
    if not paths:
        return Scope(**dict.fromkeys(flags, True))
    for path in paths:
        flags["backend"] |= path.endswith(".go")
        flags["docs"] |= path.endswith(".md")
        if path.startswith("services/scrapling-fetch/"):
            flags["scrapling_fetch"] = True
        elif path.startswith("extension/"):
            flags["extension"] = True
        elif path.startswith(("typespec/", "openapi/", "internal/api/apigen/")):
            flags.update(backend=True, web=True, contracts=True)
        elif path.startswith(("web/", "design-system/")):
            flags["web"] = True
            flags["design_system"] |= path.startswith("web/src/")
            flags["contracts"] |= (
                path.startswith(
                    ("web/src/lib/generated/", "web/src/lib/docs/generated/")
                )
                or path == "web/static/openapi.yaml"
            )
            if path.startswith("design-system/"):
                flags.update(design_system=True, extension=True)
        elif path.startswith(("internal/", "cmd/", "migrations/")) or path in (
            "go.mod",
            "go.sum",
            ".golangci.yml",
        ):
            flags["backend"] = True
            flags["contracts"] = True
            flags["migrations"] |= path.startswith("migrations/")
            flags["sqlc"] |= path.startswith(("migrations/", "internal/platform/db/"))
            flags["web"] |= path.startswith("internal/ai/plan/")
        elif path in ("docs/API.md", "docs/API.internal.md"):
            flags.update(web=True, contracts=True)
        elif path.startswith(("docs/", "openspec/")) or (
            "/" not in path and path.endswith(".md")
        ):
            flags["docs"] = True
        else:
            # Shared configuration and unfamiliar inputs select every check.
            return Scope(**dict.fromkeys(flags, True))
    return Scope(**flags)


def read_changes(base, head, repo=Path()):
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
            capture_output=True,
            timeout=60,
        )
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        print(
            "Cannot determine changed files; selecting the full CI suite.",
            file=sys.stderr,
        )
        return None
    return [os.fsdecode(path) for path in result.stdout.split(b"\0") if path]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True)
    parser.add_argument("--head", required=True)
    args = parser.parse_args()
    scope = classify(read_changes(args.base, args.head))
    print(f"backend_tasks={json.dumps(scope.backend_tasks)}")
    for name in Scope.__dataclass_fields__:
        print(f"{name}={str(getattr(scope, name)).lower()}")


if __name__ == "__main__":
    main()
