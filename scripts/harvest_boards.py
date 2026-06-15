#!/usr/bin/env python3
"""Harvest ATS board slugs from public job aggregators (every provider in VALIDATORS).

Pipeline: collect candidate (provider, slug, company) tuples from a set of
aggregator JSON files on GitHub (and, optionally, GitHub code search), drop the
ones we already track in sources/*.yml, validate the rest against the same public
ATS endpoints our ingest adapters use, and print the survivors as ready-to-paste
YAML.

Usage:
    python3 scripts/harvest_boards.py              # JSON aggregators only
    python3 scripts/harvest_boards.py --github     # also sweep GitHub code search (needs gh, 10 req/min)
    python3 scripts/harvest_boards.py --write      # append survivors to sources/<provider>.yml

Only standard library is used; GitHub code search shells out to `gh`.
"""

from __future__ import annotations

import argparse
import html
import json
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from ats_boards import (  # noqa: E402
    REPO, SOURCES_DIR, UA, SLUG_PATTERNS, SLUG_BLOCKLIST, VALIDATORS,
    fetch, yaml_name, extract_slugs, existing_slugs, validate,
    github_fragments, emit_survivors,
)

# Aggregator JSON files. We sweep the raw text with regex, so the per-file schema
# (key names) does not matter — only that ATS URLs appear somewhere in the JSON.
AGGREGATORS = [
    "https://raw.githubusercontent.com/vanshb03/Summer2026-Internships/dev/.github/scripts/listings.json",
    "https://raw.githubusercontent.com/SimplifyJobs/Summer2026-Internships/dev/.github/scripts/listings.json",
    "https://raw.githubusercontent.com/crypto-jobs-fyi/crawler/HEAD/ai_companies.json",
    "https://raw.githubusercontent.com/crypto-jobs-fyi/crawler/HEAD/crypto_companies.json",
    "https://raw.githubusercontent.com/crypto-jobs-fyi/crawler/HEAD/fin_companies.json",
    "https://raw.githubusercontent.com/crypto-jobs-fyi/crawler/HEAD/tech_companies.json",
]

# Workday is a separate beast: a board is host + career-site path (e.g.
# "logitech.wd5.myworkdayjobs.com/Logitech"), and the site segment sits before
# /job/ or /details/ in a posting URL, after an optional locale prefix.
WORKDAY_RE = re.compile(
    r"https?://([a-z0-9-]+\.wd\d+\.myworkdayjobs\.com)/(?:[a-zA-Z]{2}-[a-zA-Z]{2}/)?([^/?\"]+)/(?:job|details)/"
)


def harvest_aggregators() -> dict[tuple[str, str], str]:
    """Return {(provider, slug): company_name}. Company name is best-effort."""
    candidates: dict[tuple[str, str], str] = {}
    for url in AGGREGATORS:
        body = fetch(url)
        if not body:
            print(f"  ! skip (fetch failed): {url}", file=sys.stderr)
            continue
        text = body.decode("utf-8", "replace")
        found = extract_slugs(text)
        # Best-effort company-name attribution from structured JSON.
        names = attribute_names(text)
        for prov, slug in found:
            candidates.setdefault((prov, slug), names.get(slug.lower(), slug))
        print(f"  {len(found):4} slugs from {url.split('/')[4]}/{url.split('/')[5]} ({url.rsplit('/',1)[-1]})", file=sys.stderr)
    return candidates


def attribute_names(text: str) -> dict[str, str]:
    """Map lowercased slug -> human company name when the JSON exposes one."""
    names: dict[str, str] = {}
    try:
        data = json.loads(text)
    except Exception:
        return names
    rows = data if isinstance(data, list) else data.values() if isinstance(data, dict) else []
    for row in rows:
        if not isinstance(row, dict):
            continue
        name = next((row[k] for k in ("company_name", "company", "name", "employer") if row.get(k)), None)
        if not name:
            continue
        for v in row.values():
            if not isinstance(v, str):
                continue
            for prov, slug in extract_slugs(v):
                names.setdefault(slug.lower(), str(name).strip())
    return names


def harvest_github(per_provider_pages: int = 3) -> set[tuple[str, str]]:
    """Sweep GitHub code search for the ATS host patterns (needs gh).

    Code search is capped at 10 req/min, so the host list is ordered with the
    rarer providers first — if the quota runs out, the well-covered greenhouse/
    lever/ashby hosts are the ones that get truncated, not the long-tail ones.
    """
    out: set[tuple[str, str]] = set()
    queries = [
        "jobs.smartrecruiters.com", "apply.workable.com", "recruitee.com",
        "job-boards.greenhouse.io", "jobs.lever.co", "jobs.ashbyhq.com",
    ]
    for q in queries:
        for page in range(1, per_provider_pages + 1):
            try:
                raw = subprocess.run(
                    ["gh", "api", "-X", "GET", "search/code",
                     "-H", "Accept: application/vnd.github.text-match+json",
                     "-f", f"q={q} in:file", "-f", "per_page=100", "-f", f"page={page}"],
                    capture_output=True, text=True, timeout=60,
                ).stdout
                items = json.loads(raw).get("items", [])
            except Exception as e:
                print(f"  ! github query failed ({q} p{page}): {e}", file=sys.stderr)
                break
            if not items:
                break
            for it in items:
                for m in it.get("text_matches", []):
                    out |= extract_slugs(m.get("fragment", ""))
    return out


def harvest_workday() -> dict[tuple[str, str], str]:
    """Return {(host, site): company_name} for Workday boards in the aggregators."""
    out: dict[tuple[str, str], str] = {}
    for url in AGGREGATORS:
        body = fetch(url)
        if not body:
            continue
        text = body.decode("utf-8", "replace")
        names = workday_names(text)
        for host, site in WORKDAY_RE.findall(text):
            tenant = host.split(".")[0]
            out.setdefault((host, site), names.get((host, site), tenant.title()))
    return out


def workday_names(text: str) -> dict[tuple[str, str], str]:
    """Best-effort (host, site) -> company name from structured JSON rows."""
    names: dict[tuple[str, str], str] = {}
    try:
        data = json.loads(text)
    except Exception:
        return names
    rows = data if isinstance(data, list) else data.values() if isinstance(data, dict) else []
    for row in rows:
        if not isinstance(row, dict):
            continue
        name = next((row[k] for k in ("company_name", "company", "name", "employer") if row.get(k)), None)
        if not name:
            continue
        for v in row.values():
            if isinstance(v, str):
                for host, site in WORKDAY_RE.findall(v):
                    names.setdefault((host, site), str(name).strip())
    return names


def harvest_github_workday(pages: int = 5) -> dict[tuple[str, str], str]:
    """Sweep GitHub code search for Workday board URLs (job lists in READMEs etc.)."""
    out: dict[tuple[str, str], str] = {}
    for frag in github_fragments("myworkdayjobs.com", pages):
        for host, site in WORKDAY_RE.findall(frag):
            out.setdefault((host, site), host.split(".")[0].title())
    return out


def existing_workday() -> set[str]:
    out: set[str] = set()
    f = SOURCES_DIR / "workday.yml"
    if f.exists():
        # CXS is case-insensitive on host AND site, so dedup on the whole board lowered.
        for m in re.findall(r"board:\s*\"?([^\"\n]+)\"?", f.read_text()):
            out.add(m.strip().lower())
    return out


def validate_workday(host: str, site: str, attempts: int = 3) -> int | None:
    """POST the CXS jobs endpoint; return posting count if the board is live.

    Retries only transient failures (timeouts, 429/5xx): the POST flakes badly
    under load and a single attempt silently drops live boards. A clean 200 (even
    with zero jobs) or a 404 is authoritative — no retry.
    """
    tenant = host.split(".")[0]
    url = f"https://{host}/wday/cxs/{tenant}/{site}/jobs"
    payload = json.dumps({"appliedFacets": {}, "limit": 20, "offset": 0, "searchText": ""}).encode()
    for attempt in range(attempts):
        req = urllib.request.Request(
            url, data=payload,
            headers={"User-Agent": UA, "Content-Type": "application/json", "Accept": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=20) as r:
                data = json.loads(r.read())
            total = data.get("total") if isinstance(data, dict) else 0
            total = total or len(data.get("jobPostings", []) if isinstance(data, dict) else [])
            return total or None
        except urllib.error.HTTPError as e:
            if e.code not in (429, 500, 502, 503, 504):
                return None  # 404 and other client errors: board is genuinely absent
        except Exception:
            pass  # timeout / connection reset: transient, retry
        if attempt < attempts - 1:
            time.sleep(1.5 * (attempt + 1))
    return None


def run_workday(write: bool, github: bool) -> int:
    print("== harvesting Workday boards ==", file=sys.stderr)
    cand = harvest_workday()
    if github:
        print("== sweeping GitHub code search (myworkdayjobs) ==", file=sys.stderr)
        for key, name in harvest_github_workday().items():
            cand.setdefault(key, name)
    have = existing_workday()
    # Collapse case-variants of one board (CXS is case-insensitive), keeping the first
    # spelling seen, and drop anything already tracked.
    new: dict[tuple[str, str], str] = {}
    for (h, s), name in cand.items():
        board = f"{h}/{s}"
        if board.lower() in have:
            continue
        have.add(board.lower())
        new[(h, s)] = name
    print(f"{len(cand)} candidates, {len(cand) - len(new)} already tracked/collapsed, "
          f"{len(new)} new to validate", file=sys.stderr)

    items = list(new.items())
    with ThreadPoolExecutor(max_workers=12) as ex:
        counts = list(ex.map(lambda kv: validate_workday(kv[0][0], kv[0][1]), items))

    rows = sorted(
        (((n, f"{h}/{s}", c) for ((h, s), n), c in zip(items, counts) if c)),
        key=lambda r: -r[2],
    )
    print(f"\n# === workday: {len(rows)} new live boards ===")
    for name, board, c in rows:
        print(f"- company: {yaml_name(name)}  # {c} jobs")
        print(f"  board: {board}")
    if write:
        f = SOURCES_DIR / "workday.yml"
        with f.open("a") as fh:
            for name, board, c in rows:
                fh.write(f"- company: {yaml_name(name)}\n  board: {board}\n")
        print(f"  -> appended {len(rows)} entries to {f.relative_to(REPO)}", file=sys.stderr)
    return 0


# ── Hacker News "Ask HN: Who is hiring?" ─────────────────────────────────────
# The monthly thread (posted by the `whoishiring` bot) is the densest public feed
# of direct ATS apply links — one recent month yields 200+ greenhouse/lever/ashby/
# workable/recruitee/bamboohr boards. Comment text is HTML-escaped (slashes as
# &#x2F;), so it MUST be unescaped before the slug regex runs.
HN_API = "https://hn.algolia.com/api/v1"


def select_hiring_threads(hits: list[dict], months: int) -> list[str]:
    """Pick the most recent N 'Who is hiring?' story ids from Algolia hits.

    Filters by title so sibling threads ('Freelancer? Seeking freelancer?',
    'Who wants to be hired?') from the same author are excluded.
    """
    ids = [h["objectID"] for h in hits if "who is hiring" in (h.get("title") or "").lower()]
    return ids[:months]


# Leading tokens that are clearly prose/role lines, not a company name. When the
# heuristic hits one, the caller falls back to a title-cased slug.
_NAME_REJECT_PREFIX = ("at ", "we ", "do you", "hiring", "join ", "about ", "our ")
_NAME_REJECT_ROLE = re.compile(
    r"(?i)^(senior|staff|principal|lead|software|backend|frontend|full[\s-]?stack|"
    r"engineer|developer|sr\.?|the )\b"
)


def _slug_title(slug: str) -> str:
    """Readable fallback name from a slug: 'acme-corp' -> 'Acme Corp'."""
    return re.sub(r"[-_.]+", " ", slug).strip().title() or slug


def hn_company_name(comment: str) -> str:
    """Best-effort company = leading token before the first separator, tags stripped.

    Who-is-hiring comments conventionally open with 'Company | role | location | …'.
    Returns "" when the leading token looks like prose/URL/role (caller falls back
    to the slug) — keeps junk like 'Do you want to work on…' out of the YAML.
    """
    text = re.sub(r"<[^>]+>", " ", comment)          # drop HTML tags
    text = html.unescape(re.sub(r"\s+", " ", text)).strip()
    head = re.split(r"[|\n\r–—:/]| - ", text, 1)[0].strip(" .,-")
    low = head.lower()
    if (not head or len(head) > 40 or "http" in low
            or low.startswith(_NAME_REJECT_PREFIX) or _NAME_REJECT_ROLE.match(head)):
        return ""
    return head


def extract_hn_candidates(comments: list[str]) -> dict[tuple[str, str], str]:
    """Per comment, attribute every ATS slug to that comment's leading company name."""
    cand: dict[tuple[str, str], str] = {}
    for c in comments:
        dec = html.unescape(c)                        # &#x2F; → / before slug regex
        name = hn_company_name(c)
        for prov, slug in extract_slugs(dec):
            cand.setdefault((prov, slug), name or _slug_title(slug))
    return cand


def hn_hiring_threads(months: int) -> list[str]:
    body = fetch(f"{HN_API}/search_by_date?tags=story,author_whoishiring&query=hiring&hitsPerPage=20")
    if not body:
        print("  ! HN thread search failed", file=sys.stderr)
        return []
    try:
        hits = json.loads(body).get("hits", [])
    except Exception:
        return []
    return select_hiring_threads(hits, months)


def hn_comment_texts(thread_id: str) -> list[str]:
    body = fetch(f"{HN_API}/items/{thread_id}", timeout=40)
    if not body:
        return []
    try:
        data = json.loads(body)
    except Exception:
        return []
    return [c.get("text") or "" for c in data.get("children", []) if c.get("text")]


def harvest_hn(months: int = 3) -> dict[tuple[str, str], str]:
    """Harvest ATS boards from the last `months` 'Who is hiring?' threads."""
    cand: dict[tuple[str, str], str] = {}
    for tid in hn_hiring_threads(months):
        comments = hn_comment_texts(tid)
        added = extract_hn_candidates(comments)
        for key, name in added.items():
            cand.setdefault(key, name)
        print(f"  HN thread {tid}: {len(comments)} comments -> {len(added)} board candidates",
              file=sys.stderr)
    return cand


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--github", action="store_true", help="also sweep GitHub code search")
    ap.add_argument("--hn", action="store_true", help="also harvest HN 'Who is hiring?' threads")
    ap.add_argument("--hn-months", type=int, default=3, help="how many recent HN threads to scan")
    ap.add_argument("--workday", action="store_true", help="harvest Workday boards only (host/site, POST validation)")
    ap.add_argument("--write", action="store_true", help="append survivors to sources/<provider>.yml")
    args = ap.parse_args()

    if args.workday:
        return run_workday(args.write, args.github)

    print("== harvesting aggregators ==", file=sys.stderr)
    cand = harvest_aggregators()
    if args.github:
        print("== sweeping GitHub code search ==", file=sys.stderr)
        for key in harvest_github():
            cand.setdefault(key, key[1])
    if args.hn:
        print(f"== harvesting HN 'Who is hiring?' ({args.hn_months} threads) ==", file=sys.stderr)
        for key, name in harvest_hn(args.hn_months).items():
            cand.setdefault(key, name)

    emit_survivors(cand, args.write)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
