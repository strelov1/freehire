#!/usr/bin/env python3
"""Query-driven open-web ATS board discovery.

Run a free-text query across search channels (DuckDuckGo, Google CSE, GitHub code
search, Common Crawl), extract ATS board URLs, dedup against sources/*.yml,
validate each board live, and print (or --write) ready-to-paste YAML.

Usage:
    python3 scripts/discover_boards.py --query "fintech berlin" \
            --provider ashby,lever --channel ddg,github,google,cc [--write] [--limit N]

Stdlib only; the github channel shells out to `gh`; google needs GOOGLE_CSE_KEY/_CX.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.parse
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from ats_boards import github_fragments, extract_slugs, emit_survivors  # noqa: E402

# Provider -> the ATS host to put in a `site:` search / Common Crawl prefix.
PROVIDER_HOSTS = {
    "greenhouse": "job-boards.greenhouse.io",
    "lever": "jobs.lever.co",
    "ashby": "jobs.ashbyhq.com",
    "smartrecruiters": "jobs.smartrecruiters.com",
    "workable": "apply.workable.com",
    "recruitee": "recruitee.com",
    "bamboohr": "bamboohr.com",
    "breezy": "breezy.hr",
    "personio": "jobs.personio.com",
    "jazzhr": "applytojob.com",
    "teamtailor": "teamtailor.com",
}

# DuckDuckGo's HTML endpoint rejects unusual UAs; use a browser-like one.
BROWSER_UA = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)


def get_text(url: str, timeout: int = 25) -> str:
    """GET a URL with a browser UA, returning decoded text ('' on failure)."""
    req = urllib.request.Request(url, headers={"User-Agent": BROWSER_UA})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.read().decode("utf-8", "replace")
    except Exception as e:
        print(f"  ! GET failed ({url[:60]}...): {e}", file=sys.stderr)
        return ""


def parse_ddg_html(html: str) -> set[str]:
    """Extract target URLs from DuckDuckGo HTML results (unwrap ?uddg= redirects)."""
    return {urllib.parse.unquote(m) for m in re.findall(r"uddg=([^\"&]+)", html)}


def channel_ddg(host: str, query: str, limit: int) -> set[str]:
    """site:<host> <query> via DuckDuckGo HTML -> raw target URLs."""
    q = urllib.parse.quote(f"site:{host} {query}")
    html = get_text(f"https://html.duckduckgo.com/html/?q={q}")
    urls = parse_ddg_html(html)
    return set(list(urls)[:limit]) if limit else urls


def parse_cse_items(obj: dict) -> set[str]:
    """Extract result links from a Google Custom Search JSON response."""
    return {it["link"] for it in obj.get("items", []) if it.get("link")}


def channel_google(host: str, query: str, limit: int) -> set[str]:
    """site:<host> <query> via Google Custom Search JSON API (env-gated)."""
    key, cx = os.environ.get("GOOGLE_CSE_KEY"), os.environ.get("GOOGLE_CSE_CX")
    if not (key and cx):
        print("  ! google channel skipped (set GOOGLE_CSE_KEY and GOOGLE_CSE_CX)", file=sys.stderr)
        return set()
    q = urllib.parse.quote(f"site:{host} {query}")
    num = min(limit, 10) if limit else 10
    body = get_text(
        f"https://www.googleapis.com/customsearch/v1?key={key}&cx={cx}&q={q}&num={num}"
    )
    try:
        return parse_cse_items(json.loads(body))
    except Exception:
        return set()


def channel_github(host: str, query: str, limit: int, pages: int = 2) -> set[str]:
    """`<query> <host>` via GitHub code search -> the matched fragments as text."""
    frags = github_fragments(f"{query} {host}", pages)
    out = set(frags)
    return set(list(out)[:limit]) if limit else out


_CC_BASE: str | None = None


def cc_api_base() -> str | None:
    """Latest Common Crawl URL-index CDX API base (cached). None if unreachable."""
    global _CC_BASE
    if _CC_BASE is None:
        body = get_text("https://index.commoncrawl.org/collinfo.json")
        try:
            _CC_BASE = json.loads(body)[0]["cdx-api"]
        except Exception:
            print("  ! common-crawl index list unreachable", file=sys.stderr)
            _CC_BASE = ""
    return _CC_BASE or None


def parse_cc_jsonl(text: str) -> set[str]:
    """Collect the `url` field from a Common Crawl CDX JSONL response."""
    urls: set[str] = set()
    for line in text.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            row = json.loads(line)
        except Exception:
            continue
        if isinstance(row, dict) and row.get("url"):
            urls.add(row["url"])
    return urls


def channel_cc(host: str, query: str, limit: int) -> set[str]:
    """Bulk-dump board URLs for <host>/* from Common Crawl. Ignores the keyword.

    Only effective for path-based providers (greenhouse/lever/ashby/smartrecruiters/
    workable). Subdomain-board providers (recruitee, bamboohr, breezy, personio,
    teamtailor) sit on <slug>.<host>, which a <host>/* prefix never matches — they
    yield zero here; use ddg/google/github for those.
    """
    base = cc_api_base()
    if not base:
        return set()
    cap = (limit or 100) * 5  # over-fetch; many rows collapse to one slug
    url = f"{base}?url={urllib.parse.quote(host)}/*&output=json&limit={cap}"
    return parse_cc_jsonl(get_text(url, timeout=60))


CHANNELS = {
    "ddg": channel_ddg,
    "google": channel_google,
    "github": channel_github,
    "cc": channel_cc,
}


def collect_candidates(providers: list[str], channels: list[str], query: str,
                       limit: int) -> dict[tuple[str, str], str]:
    """Run each channel for each provider; return {(provider, slug): slug}.

    Results are filtered to the queried provider so noise from other ATS links on a
    results page is dropped. Company name is best-effort (the slug) for web sources.
    """
    cand: dict[tuple[str, str], str] = {}
    for provider in providers:
        host = PROVIDER_HOSTS[provider]
        for ch in channels:
            urls = CHANNELS[ch](host, query, limit)
            found = set()
            for u in urls:
                found |= extract_slugs(u)
            keep = {(p, s) for (p, s) in found if p == provider}
            for p, s in keep:
                cand.setdefault((p, s), s)
            print(f"  [{ch}/{provider}] {len(urls)} urls -> {len(keep)} {provider} slugs",
                  file=sys.stderr)
    return cand


def main() -> int:
    ap = argparse.ArgumentParser(description="Query-driven ATS board discovery")
    ap.add_argument("--query", default="", help="search term for this run")
    ap.add_argument("--provider", default="", help="comma list; default = all")
    ap.add_argument("--channel", default="ddg", help="comma list from ddg,google,github,cc")
    ap.add_argument("--write", action="store_true", help="append survivors to sources/<provider>.yml")
    ap.add_argument("--limit", type=int, default=20, help="cap results per channel/provider")
    args = ap.parse_args()

    providers = [p.strip() for p in args.provider.split(",") if p.strip()] or list(PROVIDER_HOSTS)
    channels = [c.strip() for c in args.channel.split(",") if c.strip()]
    bad_p = [p for p in providers if p not in PROVIDER_HOSTS]
    bad_c = [c for c in channels if c not in CHANNELS]
    if bad_p:
        ap.error(f"unknown provider(s): {bad_p}; choose from {list(PROVIDER_HOSTS)}")
    if bad_c:
        ap.error(f"unknown channel(s): {bad_c}; choose from {list(CHANNELS)}")
    if not args.query and channels != ["cc"]:
        ap.error("--query is required for keyword channels (ddg/google/github)")

    print(f"== discovering: query={args.query!r} providers={providers} channels={channels} ==",
          file=sys.stderr)
    cand = collect_candidates(providers, channels, args.query, args.limit)
    emit_survivors(cand, args.write)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
