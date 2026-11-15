#!/usr/bin/env python3
"""Add the two verified Clinch destinations; preserve PageUp boards and all jobs."""

import argparse
import json
import os
import shlex
import subprocess

DESTINATIONS = {
    "893": "careers.madergroup.com.au",
    "784": "armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com",
}


def additions(rows):
    commands = []
    for old, host in DESTINATIONS.items():
        origins = [r for r in rows if r["provider"] == "pageup" and r["board"] == old]
        if len(origins) != 1:
            raise ValueError(f"expected one live PageUp board {old}")
        origin = origins[0]
        if origin["region"] or origin["hub"] or origin["tenants"]:
            raise ValueError(
                f"unexpected metadata on PageUp board {old}; review manually"
            )
        targets = [
            r for r in rows if r["provider"] == "clinch" and r["board"].lower() == host
        ]
        if targets:
            if len(targets) != 1 or any(
                targets[0][k] != origin[k]
                for k in ("company", "region", "hub", "tenants")
            ):
                raise ValueError(
                    f"destination {host} has conflicting identity metadata"
                )
            continue
        commands.append(
            [
                "go",
                "run",
                "./cmd/add-board",
                "--provider=clinch",
                f"--board={host}",
                f"--company={origin['company']}",
            ]
        )
    return commands


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--apply", action="store_true", help="add missing destinations only"
    )
    args = parser.parse_args()
    query = """BEGIN READ ONLY;
SELECT coalesce(json_agg(b), '[]'::json) FROM (
 SELECT provider, board, region, company, hub, tenants FROM boards
 WHERE status IN ('active', 'pending') AND (
  (provider = 'pageup' AND board IN ('893', '784')) OR
  (provider = 'clinch' AND lower(board) IN (
   'careers.madergroup.com.au',
   'armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com')))
) b;
COMMIT;"""
    result = subprocess.run(
        [
            "psql",
            "--dbname",
            os.environ["DATABASE_URL"],
            "-X",
            "-qAt",
            "-v",
            "ON_ERROR_STOP=1",
            "-c",
            query,
        ],
        capture_output=True,
        text=True,
    )
    if result.returncode:
        raise RuntimeError("catalog read-only preflight failed; check database access")
    commands = additions(json.loads(result.stdout))
    for command in commands:
        print(shlex.join(command + (["--apply"] if args.apply else [])), flush=True)
        if args.apply:
            subprocess.run(command + ["--apply"], check=True)
    print(f"{len(commands)} missing destinations; PageUp boards and jobs untouched")


if __name__ == "__main__":
    main()
