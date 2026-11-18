#!/usr/bin/env python3
"""Add the 38 verified Clinch destinations; preserve PageUp boards and all jobs."""

import argparse
import json
import os
import shlex
import subprocess

DESTINATIONS = {
    "893": "careers.madergroup.com.au",
    "784": "armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com",
    "743": "jobs.citadel.edu",
    "1041": "careers.dha.gov.au",
    "1081": "careers.savills.com.au",
    "885": "jobs.du.edu",
    "710": "psacareers.singaporepsa.com",
    "992": "careers.wintv.com.au",
    "978": "jobs.boisestate.edu",
    "988": "careers.duratec.com.au",
    "870": "careers.marist.edu",
    "998": "careers.mater.org.au",
    "825": "careers.ahpra.gov.au",
    "586": "careers.moretonbay.qld.gov.au",
    "960": "careers.northline.com.au",
    "924": "external-jobs.csu.edu.au",
    "1027": "north-idaho-college-careers.career-pages.com",
    "820": "careers.drexel.edu",
    "902": "careers.artscentremelbourne.com.au",
    "1072": "careers.whiddon.com.au",
    "1000": "careers.knox.vic.gov.au",
    "449": "careers.linfox.com",
    "867": "jobs.sandiego.edu",
    "987": "jobs.yooralla.com.au",
    "709": "career.leightonasia.com",
    "530": "careers.worldvision.com.au",
    "923": "careers.utas.edu.au",
    "951": "jobs.amanaliving.com.au",
    "1080": "schindler-lifts-australia-careers.careerpages.rec-marketing.dc2.pageuppeople.com",
    "1098": "careers.device.com.au",
    "671": "jobs.unicef.org",
    "695": "careers.kenyon.edu",
    "724": "careers.lwb.org.au",
    "1138": "careers.fire.nsw.gov.au",
    "1155": "careers.tafensw.edu.au",
    "1197": "careers.ses.nsw.gov.au",
    "1199": "careers.rfs.nsw.gov.au",
    "1192": "careers.icac.nsw.gov.au",
}
EXPECTED_COMPANIES = {
    "893": "Mader Group",
    "784": "Armaguard Group",
    "743": "The Citadel",
    "1041": "Defence Housing Australia",
    "1081": "Savills",
    "885": "University of Denver",
    "710": "PSA International",
    "992": "WIN Network",
    "978": "Boise State University",
    "988": "Duratec",
    "870": "Marist University",
    "998": "Mater",
    "825": "Australian Health Practitioner Regulation Agency",
    "586": "City of Moreton Bay",
    "960": "Northline",
    "924": "Charles Sturt University",
    "1027": "North Idaho College",
    "820": "Drexel University",
    "902": "Arts Centre Melbourne",
    "1072": "Whiddon",
    "1000": "Knox City Council",
    "449": "Linfox",
    "867": "University of San Diego",
    "987": "Berry Street Yooralla",
    "709": "CIMIC Group",
    "530": "World Vision Australia",
    "923": "University of Tasmania",
    "951": "Amana Living",
    "1080": "Schindler Lifts",
    "1098": "Device Technologies",
    "671": "United Nations Children's Fund (UNICEF)",
    "695": "Kenyon College",
    "724": "Life Without Barriers",
    "1138": "Fire and Rescue NSW",
    "1155": "TAFE NSW",
    "1197": "NSW State Emergency Service",
    "1199": "NSW Rural Fire Service",
    "1192": "NSW Independent Commission Against Corruption",
}


def additions(rows):
    commands = []
    for old, host in DESTINATIONS.items():
        origins = [r for r in rows if r["provider"] == "pageup" and r["board"] == old]
        if len(origins) != 1:
            raise ValueError(f"expected one live PageUp board {old}")
        origin = origins[0]
        if (
            origin["company"] != EXPECTED_COMPANIES[old]
            or origin["region"]
            or origin["hub"]
            or origin["tenants"]
        ):
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


def sql_literal(value):
    return "'" + value.replace("'", "''") + "'"


def preflight_query():
    origins = ", ".join(map(sql_literal, DESTINATIONS))
    hosts = ", ".join(map(sql_literal, DESTINATIONS.values()))
    return f"""BEGIN READ ONLY;
SELECT coalesce(json_agg(b), '[]'::json) FROM (
 SELECT provider, board, region, company, hub, tenants FROM boards
 WHERE status IN ('active', 'pending') AND (
  (provider = 'pageup' AND board IN ({origins})) OR
  (provider = 'clinch' AND lower(board) IN ({hosts})))
) b;
COMMIT;"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--apply", action="store_true", help="add missing destinations only"
    )
    args = parser.parse_args()
    query = preflight_query()
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
            subprocess.run([*command, "--apply"], check=True)
    print(f"{len(commands)} missing destinations; PageUp boards and jobs untouched")


if __name__ == "__main__":
    main()
