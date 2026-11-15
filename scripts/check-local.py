"""Explain and optionally execute local checks for an explicit base plus dirty files."""

import argparse
import importlib.util
import os
import shlex
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location(
    "ci_changes", ROOT / ".github/scripts/ci_changes.py"
)
ci = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = ci
spec.loader.exec_module(ci)


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)


def changes(base):
    """Disable rename detection so both endpoints participate in classification."""
    resolved = git("rev-parse", "--verify", f"{base}^{{commit}}").decode().strip()
    paths = set()
    for args in (
        ("diff", "--no-renames", "--name-only", "-z", resolved, "HEAD", "--"),
        ("diff", "--no-renames", "--name-only", "-z", "--"),
        ("diff", "--cached", "--no-renames", "--name-only", "-z", "--"),
        ("ls-files", "--others", "--exclude-standard", "-z"),
    ):
        paths.update(os.fsdecode(p) for p in git(*args).split(b"\0") if p)
    return resolved, sorted(paths)


def plan(paths):
    scope = ci.classify(paths)
    commands = [["pnpm", "check:links"], ["pnpm", "check:size"]]
    # Hooks own file-specific rules; passing paths also includes untracked edits.
    if paths:
        commands.append(
            [
                "lefthook",
                "run",
                "pre-commit",
                *[arg for path in paths for arg in ("--file", path)],
            ]
        )
    else:
        commands.append(["lefthook", "run", "pre-commit", "--all-files"])
    if scope.backend:
        commands.extend(
            shlex.split(command)
            for command in (
                "go build ./...",
                "go vet ./...",
                "go test ./...",
                "go vet -tags=integration ./...",
            )
        )
    else:
        commands.append(["go", "test", "./..."])
    if scope.web:
        commands.extend(
            ["pnpm", "--dir", "web", "run", task]
            for task in ("format:check", "lint", "check", "test")
        )
    return scope, commands


def execute(commands):
    failed = False
    for command in commands:
        print(f"RUN {shlex.join(command)}", flush=True)
        try:
            result = subprocess.run(command, cwd=ROOT, check=False)
            code = result.returncode
        except OSError as error:
            print(error, file=sys.stderr)
            code = 127
        print(
            f"{'FAIL' if code else 'PASS'} ({code}): {shlex.join(command)}", flush=True
        )
        failed |= code != 0
    return int(failed)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True, help="exact comparison commit or ref")
    parser.add_argument("--run", action="store_true")
    args = parser.parse_args()
    try:
        base, paths = changes(args.base)
        print(f"Base: {args.base} -> {base}")
    except (OSError, subprocess.CalledProcessError) as error:
        print(f"Unknown change input ({error}); full fallback")
        paths = None
    print("Dirty status (index/worktree/untracked):")
    print(git("status", "--short").decode(), end="")
    print(f"Changed paths: {paths!r}")
    scope, commands = plan(paths)
    print(f"CI classifier: backend={scope.backend}, web={scope.web}")
    print(
        "Local evidence only; not final readiness. CI and behavior integration tests remain required."
    )
    for command in commands:
        print(f"PLAN {shlex.join(command)}")
    return execute(commands) if args.run else 0


if __name__ == "__main__":
    sys.exit(main())
