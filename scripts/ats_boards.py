#!/usr/bin/env python3
"""Shared ATS board core: slug extraction, live validation, dedup, YAML emit.

Used by both scripts/harvest_boards.py (aggregator-driven) and
scripts/discover_boards.py (query-driven web discovery). Stdlib only.
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
import urllib.request
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
SOURCES_DIR = REPO / "sources"
UA = "freehire-harvest/1.0 (+https://freehire.dev)"

# (compiled regex, provider). Group 1 (first non-empty group) is the board slug.
SLUG_PATTERNS = [
    (re.compile(r"(?:boards|job-boards)(?:\.eu)?\.greenhouse\.io/(?:embed/job_app\?for=)?([A-Za-z0-9_-]+)"), "greenhouse"),
    (re.compile(r"jobs\.(?:eu\.)?lever\.co/([A-Za-z0-9_.-]+)"), "lever"),
    (re.compile(r"jobs\.ashbyhq\.com/([A-Za-z0-9_.-]+)"), "ashby"),
    (re.compile(r"(?:jobs|careers)\.smartrecruiters\.com/([A-Za-z0-9_-]+)|api\.smartrecruiters\.com/v1/companies/([A-Za-z0-9_-]+)"), "smartrecruiters"),
    (re.compile(r"apply\.workable\.com/([A-Za-z0-9_-]+)"), "workable"),
    (re.compile(r"([A-Za-z0-9_-]+)\.recruitee\.com"), "recruitee"),
    (re.compile(r"([A-Za-z0-9_-]+)\.bamboohr\.com"), "bamboohr"),
    (re.compile(r"([A-Za-z0-9_-]+)\.breezy\.hr"), "breezy"),
    (re.compile(r"([A-Za-z0-9_-]+)\.jobs\.personio\.(?:com|de)"), "personio"),
    (re.compile(r"([A-Za-z0-9_-]+)\.applytojob\.com"), "jazzhr"),
    # Teamtailor's "slug" is the whole board host — the adapter takes board = hostname.
    # Only *.teamtailor.com hosts are detectable here; boards on a custom domain
    # (e.g. jobs.tibber.com) carry no teamtailor marker in the URL and are missed.
    (re.compile(r"([A-Za-z0-9_-]+\.teamtailor\.com)"), "teamtailor"),
]

SLUG_BLOCKLIST = {
    "embed", "jobs", "job", "j", "o", "share", "en-us", "en-gb",
    "api", "widget", "backend", "www", "app", "auth", "referrals", "v1",
}

# Validation endpoints — each identical to the one its internal/sources/<provider>.go
# adapter calls. bamboohr returns JSON, personio XML, teamtailor HTML (see validate()).
VALIDATORS = {
    "greenhouse": lambda s: f"https://boards-api.greenhouse.io/v1/boards/{s}/jobs?content=true",
    "lever": lambda s: f"https://api.lever.co/v0/postings/{s}?mode=json",
    "ashby": lambda s: f"https://api.ashbyhq.com/posting-api/job-board/{s}",
    "smartrecruiters": lambda s: f"https://api.smartrecruiters.com/v1/companies/{s}/postings?limit=10",
    "workable": lambda s: f"https://apply.workable.com/api/v1/widget/accounts/{s}?details=true",
    "recruitee": lambda s: f"https://{s}.recruitee.com/api/offers/",
    "bamboohr": lambda s: f"https://{s}.bamboohr.com/careers/list",
    "breezy": lambda s: f"https://{s}.breezy.hr/json",  # top-level JSON array of positions
    "personio": lambda s: f"https://{s}.jobs.personio.com/xml",
    "jazzhr": lambda s: f"https://{s}.applytojob.com/apply",  # /apply listing HTML
    "teamtailor": lambda s: f"https://{s}/jobs",  # s is the board host, not a slug
}


def fetch(url: str, timeout: int = 25) -> bytes | None:
    req = urllib.request.Request(url, headers={"User-Agent": UA, "Accept": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.read()
    except Exception:
        return None


def yaml_name(name: str) -> str:
    """Quote a company name only when it carries YAML-significant characters."""
    name = name.strip()
    if not name or name[0] in "-?:,[]{}#&*!|>'\"%@`" or any(c in name for c in ':#,"'):
        return '"' + name.replace('\\', '\\\\').replace('"', '\\"') + '"'
    return name


def extract_slugs(text: str) -> set[tuple[str, str]]:
    """Sweep raw text for ATS board URLs -> {(provider, slug)}.

    Patterns may carry multiple alternation groups (e.g. SmartRecruiters' two host
    forms), so take the first non-empty group of each match as the slug.
    """
    out: set[tuple[str, str]] = set()
    for pat, provider in SLUG_PATTERNS:
        for m in pat.finditer(text):
            slug = next((g for g in m.groups() if g), None)
            if not slug or slug.lower() in SLUG_BLOCKLIST:
                continue
            out.add((provider, slug))
    return out


def existing_slugs() -> dict[str, set[str]]:
    out: dict[str, set[str]] = defaultdict(set)
    for prov in VALIDATORS:
        f = SOURCES_DIR / f"{prov}.yml"
        if f.exists():
            for m in re.findall(r"board:\s*\"?([^\"\n]+)\"?", f.read_text()):
                out[prov].add(m.strip().lower())
    return out


def validate(provider: str, slug: str) -> int | None:
    """Return active job count if the board is live and non-empty, else None."""
    body = fetch(VALIDATORS[provider](slug), timeout=20)
    if not body:
        return None
    if provider == "personio":
        return body.count(b"<position>") or None
    if provider == "teamtailor":
        return len(set(re.findall(rb"/jobs/(\d+)", body))) or None
    if provider == "jazzhr":
        # Mirror internal/sources/jazzhr.go: a job is an /apply/<token>/ permalink.
        return len(set(re.findall(rb"/apply/([A-Za-z0-9]+)/", body))) or None
    try:
        data = json.loads(body)
    except Exception:
        return None
    d = data if isinstance(data, dict) else {}
    if provider in ("lever", "breezy"):
        count = len(data) if isinstance(data, list) else 0
    elif provider == "smartrecruiters":
        count = d.get("totalFound") or len(d.get("content", []))
    elif provider == "recruitee":
        count = len(d.get("offers", []))
    elif provider == "bamboohr":
        count = len(d.get("result", []))
    else:
        count = len(d.get("jobs", []))
    return count or None


def github_fragments(query: str, pages: int) -> list[str]:
    """Run a GitHub code-search query and return the matched text fragments."""
    frags: list[str] = []
    for page in range(1, pages + 1):
        try:
            raw = subprocess.run(
                ["gh", "api", "-X", "GET", "search/code",
                 "-H", "Accept: application/vnd.github.text-match+json",
                 "-f", f"q={query} in:file", "-f", "per_page=100", "-f", f"page={page}"],
                capture_output=True, text=True, timeout=60,
            ).stdout
            items = json.loads(raw).get("items", [])
        except Exception as e:
            print(f"  ! github query failed ({query} p{page}): {e}", file=sys.stderr)
            break
        if not items:
            break
        for it in items:
            for m in it.get("text_matches", []):
                frags.append(m.get("fragment", ""))
    return frags


def emit_survivors(cand: dict[tuple[str, str], str], write: bool) -> int:
    """Dedup vs sources/*.yml, validate live concurrently, print/append YAML.

    cand maps (provider, slug) -> best-effort company name. Returns the count of
    new validated boards. Shared by harvest (aggregators) and discover (web search).
    """
    have = existing_slugs()
    new = {(p, s): n for (p, s), n in cand.items() if s.lower() not in have[p]}
    print(f"\n{len(cand)} unique candidates, {len(cand) - len(new)} already tracked, "
          f"{len(new)} new to validate\n", file=sys.stderr)

    items = list(new.items())
    with ThreadPoolExecutor(max_workers=16) as ex:
        counts = list(ex.map(lambda kv: validate(kv[0][0], kv[0][1]), items))

    best: dict[tuple[str, str], tuple[str, str, int]] = {}
    for ((prov, slug), name), n in zip(items, counts):
        if not n:
            continue
        key = (prov, slug.lower())
        if key not in best or n > best[key][2]:
            best[key] = (name, slug, n)
    survivors: dict[str, list[tuple[str, str, int]]] = defaultdict(list)
    for (prov, _), row in best.items():
        survivors[prov].append(row)

    total = 0
    for prov in VALIDATORS:
        rows = sorted(survivors[prov], key=lambda r: -r[2])
        if not rows:
            continue
        total += len(rows)
        print(f"\n# === {prov}: {len(rows)} new live boards ===")
        for name, slug, n in rows:
            print(f"- company: {yaml_name(name)}  # {n} jobs")
            print(f"  board: {slug}")
        if write:
            f = SOURCES_DIR / f"{prov}.yml"
            with f.open("a") as fh:
                for name, slug, n in rows:
                    fh.write(f"- company: {yaml_name(name)}\n  board: {slug}\n")
            print(f"  -> appended {len(rows)} entries to {f.relative_to(REPO)}", file=sys.stderr)

    print(f"\n{total} new validated boards total", file=sys.stderr)
    return total
