#!/usr/bin/env python3
"""Plain-assert tests for mine_aggregators harvesters (no pytest, stdlib only, no network).
Run: python3 scripts/test_mine_aggregators.py"""

import sys
import traceback
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import mine_aggregators as m  # noqa: E402


def _router(routes):
    """Return a get_json stub that dispatches by substring match against routes, counting calls."""
    calls = {"n": 0}

    def get_json(url):
        calls["n"] += 1
        for frag, payload in routes.items():
            if frag in url:
                return payload
        return None

    return get_json, calls


def test_rows_candidates_attributes_company_from_applyurl():
    rows = [{"companyName": "Grape Up", "applyUrl": "https://job-boards.greenhouse.io/grapeup"}]
    cand = m._rows_candidates(rows)
    assert cand[("greenhouse", "grapeup")] == "Grape Up"


def test_rows_candidates_falls_back_to_slug_without_name():
    rows = [{"applyUrl": "https://jobs.lever.co/standalone"}]
    cand = m._rows_candidates(rows)
    assert cand[("lever", "standalone")] == "standalone"


def test_rows_candidates_first_row_wins_attribution():
    rows = [
        {"companyName": "Acme", "applyUrl": "https://jobs.ashbyhq.com/acme"},
        {"companyName": "", "applyUrl": "https://jobs.ashbyhq.com/acme"},
    ]
    assert m._rows_candidates(rows)[("ashby", "acme")] == "Acme"


def test_harvest_justjoin_follows_detail_applyurl():
    get_json, calls = _router({
        "by-cursor": {"data": [{"slug": "acme-dev", "companyName": "Acme"}], "meta": {"next": None}},
        "/v1/offers/acme-dev": {"applyUrl": "https://apply.workable.com/acme"},
    })
    cand = m.harvest_justjoin(limit=10, get_json=get_json, max_workers=1)
    assert cand[("workable", "acme")] == "Acme"
    # one list page + one detail fetch for the single offer
    assert calls["n"] == 2, calls["n"]


def test_harvest_justjoin_limit_bounds_detail_fetches():
    # A feed of 5 offers across cursor pages, but limit=2 must fetch only 2 details.
    page1 = {"data": [{"slug": f"s{i}", "companyName": f"C{i}"} for i in range(3)],
             "meta": {"next": {"cursor": 9}}}
    page2 = {"data": [{"slug": f"s{i}", "companyName": f"C{i}"} for i in range(3, 5)],
             "meta": {"next": None}}
    seq = {"n": 0}

    def get_json(url):
        if "by-cursor" in url and "from=" not in url:
            return page1
        if "from=9" in url:
            return page2
        return {"applyUrl": ""}  # detail

    cand = m.harvest_justjoin(limit=2, get_json=get_json, max_workers=1)
    assert cand == {}  # empty applyUrls -> no boards, but must not crash


def test_harvest_arbeitnow_sweeps_description_body():
    get_json, calls = _router({
        "page=1": {"data": [{"company_name": "Foo Inc",
                             "description": 'apply at <a href="https://jobs.lever.co/fooinc">x</a>'}]},
        "page=2": {"data": []},
    })
    cand = m.harvest_arbeitnow(limit=100, get_json=get_json)
    assert cand[("lever", "fooinc")] == "Foo Inc"


def test_harvest_arbeitnow_stops_on_empty_page():
    get_json, calls = _router({"page=1": {"data": []}})
    assert m.harvest_arbeitnow(limit=100, get_json=get_json) == {}
    assert calls["n"] == 1


def _run():
    fns = [v for k, v in sorted(globals().items()) if k.startswith("test_") and callable(v)]
    failed = 0
    for fn in fns:
        try:
            fn()
            print(f"ok   {fn.__name__}")
        except Exception:
            failed += 1
            print(f"FAIL {fn.__name__}")
            traceback.print_exc()
    print(f"\n{len(fns) - failed}/{len(fns)} passed")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(_run())
