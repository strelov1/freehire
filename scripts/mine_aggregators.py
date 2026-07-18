#!/usr/bin/env python3
"""Mine ATS board slugs from the outbound apply-URLs of job aggregators we already crawl.

Where harvest_boards.py reads static ATS-slug dumps and discover_boards.py runs web
search, this script exploits a third signal: an aggregator lists a company's posting but
its *apply* link points straight at that company's own ATS board (justjoin's applyUrl ->
grapeup.traffit.com, arbeitnow's posting body -> job-boards.greenhouse.io/<slug>). Follow
those links, extract (provider, slug), and we discover boards without ever probing an ATS
platform ourselves.

Each aggregator reduces to a list of "rows" — dicts carrying a company name plus whatever
string fields may hide an ATS URL — and the shared _rows_candidates() sweeps them with the
same extract_slugs() regexes the other two scripts use. From there the ats_boards core
(dedup vs sources/*.yml, live validation, YAML emit) is reused verbatim.

Usage:
    python3 scripts/mine_aggregators.py                          # both aggregators, default limit
    python3 scripts/mine_aggregators.py --aggregator justjoin    # one aggregator
    python3 scripts/mine_aggregators.py --limit 500 --write      # deeper crawl, append survivors
    python3 scripts/mine_aggregators.py --provider greenhouse,lever   # only emit these providers

Stdlib only. The board coverage equals ats_boards.VALIDATORS — an applyUrl to an ATS we
have no pattern for (e.g. traffit) is silently dropped; adding one is a line in ats_boards.
"""

from __future__ import annotations

import argparse
import json
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from ats_boards import fetch, extract_slugs, emit_survivors  # noqa: E402

# Company-name keys, ordered by how the aggregators spell them (justjoin: companyName,
# arbeitnow: company_name). First non-empty wins; a row with none falls back to the slug.
NAME_KEYS = ("companyName", "company_name", "company", "name", "employer")

JUSTJOIN_LIST = "https://api.justjoin.it/v2/user-panel/offers/by-cursor"
JUSTJOIN_DETAIL = "https://api.justjoin.it/v1/offers/%s"  # carries applyUrl the list omits
ARBEITNOW_API = "https://www.arbeitnow.com/api/job-board-api"


def _get_json(url: str) -> dict | list | None:
    """GET a URL and parse JSON, returning None on any transport or decode failure."""
    body = fetch(url)
    if body is None:
        return None
    try:
        return json.loads(body)
    except Exception:
        return None


def _rows_candidates(rows: list[dict]) -> dict[tuple[str, str], str]:
    """Sweep aggregator rows for ATS boards -> {(provider, slug): company_name}.

    A row's company name is attributed to every board slug found in its string fields; the
    first row to surface a (provider, slug) wins the attribution, so an earlier, richer row
    is not overwritten by a later bare one.
    """
    cand: dict[tuple[str, str], str] = {}
    for row in rows:
        if not isinstance(row, dict):
            continue
        name = next((str(row[k]).strip() for k in NAME_KEYS if row.get(k)), "")
        blob = " ".join(v for v in row.values() if isinstance(v, str))
        for prov, slug in extract_slugs(blob):
            cand.setdefault((prov, slug), name or slug)
    return cand


def harvest_justjoin(limit: int, get_json=_get_json, max_workers: int = 8) -> dict[tuple[str, str], str]:
    """Page justjoin's cursor feed, then fetch each offer's detail for its applyUrl.

    The list omits the apply link, so the ATS is only reachable via GET /v1/offers/{slug} —
    one request per offer, which is why limit bounds the number of detail fetches (the
    expensive part), not just the listing walk.
    """
    offers: list[dict] = []
    cursor = 0
    while len(offers) < limit:
        url = JUSTJOIN_LIST if cursor == 0 else f"{JUSTJOIN_LIST}?from={cursor}"
        resp = get_json(url)
        if not isinstance(resp, dict):
            break
        offers.extend(resp.get("data") or [])
        nxt = (resp.get("meta") or {}).get("next")
        ncur = nxt.get("cursor") if isinstance(nxt, dict) else None
        # Stop at feed end or a non-advancing cursor (the `from` param was ignored) — the
        # same guard the Go adapter uses to avoid refetching page 1 forever.
        if not ncur or ncur <= cursor:
            break
        cursor = ncur
    offers = offers[:limit]

    def one(o: dict) -> dict:
        slug = o.get("slug")
        detail = get_json(JUSTJOIN_DETAIL % slug) if slug else None
        apply_url = detail.get("applyUrl") if isinstance(detail, dict) else ""
        return {"companyName": o.get("companyName", ""), "applyUrl": apply_url or ""}

    with ThreadPoolExecutor(max_workers=max_workers) as ex:
        rows = list(ex.map(one, offers))
    print(f"  justjoin: {len(offers)} offers inspected", file=sys.stderr)
    return _rows_candidates(rows)


def harvest_arbeitnow(limit: int, get_json=_get_json) -> dict[tuple[str, str], str]:
    """Page arbeitnow's public feed; the ATS link lives in the posting body, not a field.

    arbeitnow re-lists jobs it sourced from greenhouse/lever/recruitee boards, so the board
    URL appears inside the description HTML — a plain regex sweep of the row surfaces it, no
    per-offer request needed.
    """
    rows: list[dict] = []
    page = 1
    while len(rows) < limit:
        resp = get_json(f"{ARBEITNOW_API}?page={page}")
        if not isinstance(resp, dict):
            break
        data = resp.get("data") or []
        if not data:
            break
        rows.extend(data)
        page += 1
    rows = rows[:limit]
    print(f"  arbeitnow: {len(rows)} postings inspected", file=sys.stderr)
    return _rows_candidates(rows)


HARVESTERS = {
    "justjoin": harvest_justjoin,
    "arbeitnow": harvest_arbeitnow,
}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--aggregator", default=",".join(HARVESTERS),
                    help="comma-separated aggregators to mine (default: all)")
    ap.add_argument("--limit", type=int, default=200,
                    help="max postings to inspect per aggregator (bounds justjoin detail fetches)")
    ap.add_argument("--provider", default="",
                    help="comma-separated ATS providers to emit (default: all)")
    ap.add_argument("--write", action="store_true", help="append survivors to sources/<provider>.yml")
    args = ap.parse_args()

    names = [n.strip() for n in args.aggregator.split(",") if n.strip()]
    unknown = [n for n in names if n not in HARVESTERS]
    if unknown:
        ap.error(f"unknown aggregator(s): {', '.join(unknown)}; known: {', '.join(HARVESTERS)}")

    cand: dict[tuple[str, str], str] = {}
    for name in names:
        print(f"# mining {name} ...", file=sys.stderr)
        for key, company in HARVESTERS[name](args.limit).items():
            cand.setdefault(key, company)

    if args.provider:
        keep = {p.strip() for p in args.provider.split(",") if p.strip()}
        cand = {(prov, slug): n for (prov, slug), n in cand.items() if prov in keep}

    emit_survivors(cand, args.write)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
