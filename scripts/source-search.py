#!/usr/bin/env python3
"""Search an explicit source scope without hiding generation inputs."""

import argparse
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument(
    "--full",
    action="store_true",
    help="Include hidden, ignored, generated and history files",
)
parser.add_argument("scope", help="Explicit directory or file to search")
parser.add_argument("pattern")
args = parser.parse_args()
command = ["rg", "--line-number"]
if args.full:
    command += ["--hidden", "--no-ignore"]
else:
    for glob in (
        "!**/AGENTS.md",
        "!**/AGENT-REFERENCE*.md",
        "!**/node_modules/**",
        "!**/vendor/**",
        "!**/.git/**",
        "!**/apigen/**",
        "!**/*.sql.go",
        "!**/pnpm-lock.yaml",
        "!**/package-lock.json",
        "!**/go.sum",
        "!**/openspec/changes/archive/**",
    ):
        command += ["--glob", glob]
raise SystemExit(subprocess.call(command + ["--", args.pattern, args.scope]))
