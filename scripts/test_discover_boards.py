#!/usr/bin/env python3
"""Plain-assert tests for discover_boards (no pytest). Run: python3 scripts/test_discover_boards.py"""

import sys
import traceback
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import ats_boards  # noqa: E402
import discover_boards as d  # noqa: E402


def test_provider_hosts_subset_of_validators():
    assert set(d.PROVIDER_HOSTS) <= set(ats_boards.VALIDATORS), \
        "every discoverable provider must have a validator"
    assert d.PROVIDER_HOSTS["ashby"] == "jobs.ashbyhq.com"


def test_parse_ddg_html_decodes_uddg_redirect():
    html = (
        '<a class="result__a" '
        'href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fjobs.ashbyhq.com%2FClipbook%2Fabc&rut=x">'
        'Clipbook</a>'
    )
    urls = d.parse_ddg_html(html)
    assert "https://jobs.ashbyhq.com/Clipbook/abc" in urls


def test_parse_ddg_html_passes_decoded_url_to_extract_slugs():
    html = '<a href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fjobs.ashbyhq.com%2FClipbook&rut=x">x</a>'
    slugs = set()
    for url in d.parse_ddg_html(html):
        slugs |= ats_boards.extract_slugs(url)
    assert ("ashby", "Clipbook") in slugs


def test_parse_cse_items_extracts_links():
    obj = {"items": [
        {"link": "https://jobs.ashbyhq.com/Clipbook/abc"},
        {"link": "https://jobs.ashbyhq.com/Other"},
    ]}
    assert d.parse_cse_items(obj) == {
        "https://jobs.ashbyhq.com/Clipbook/abc",
        "https://jobs.ashbyhq.com/Other",
    }


def test_parse_cse_items_empty_on_no_items():
    assert d.parse_cse_items({}) == set()


def test_parse_serping_items_extracts_links():
    obj = {"organic": [
        {"title": "x", "link": "https://jobs.ashbyhq.com/Clipbook/abc", "position": 1},
        {"title": "y", "link": "https://jobs.ashbyhq.com/Other", "position": 2},
    ]}
    assert d.parse_serping_items(obj) == {
        "https://jobs.ashbyhq.com/Clipbook/abc",
        "https://jobs.ashbyhq.com/Other",
    }


def test_parse_serping_items_empty_on_no_organic():
    assert d.parse_serping_items({}) == set()


def test_channel_github_extracts_from_fragments():
    import discover_boards as dd
    orig = dd.github_fragments
    dd.github_fragments = lambda query, pages: ["see https://jobs.ashbyhq.com/Clipbook here"]
    try:
        urls = dd.channel_github("jobs.ashbyhq.com", "fintech", limit=10)
    finally:
        dd.github_fragments = orig
    assert any("jobs.ashbyhq.com/Clipbook" in u for u in urls)


def test_parse_cc_jsonl_collects_urls_and_skips_garbage():
    text = (
        '{"url":"https://jobs.ashbyhq.com/Clipbook/abc","status":"200"}\n'
        '{"url":"https://jobs.ashbyhq.com/Other"}\n'
        'not-json-line\n'
    )
    urls = d.parse_cc_jsonl(text)
    assert urls == {
        "https://jobs.ashbyhq.com/Clipbook/abc",
        "https://jobs.ashbyhq.com/Other",
    }


def test_collect_candidates_filters_to_provider():
    import discover_boards as dd
    saved = dd.CHANNELS.copy()
    # ashby search accidentally also surfaces a lever URL; keep only ashby for the ashby host.
    dd.CHANNELS = {
        "stub": lambda host, query, limit: {
            "https://jobs.ashbyhq.com/Clipbook",
            "https://jobs.lever.co/strayco",
        }
    }
    try:
        cand = dd.collect_candidates(["ashby"], ["stub"], "anything", limit=10)
    finally:
        dd.CHANNELS = saved
    assert ("ashby", "Clipbook") in cand
    assert ("lever", "strayco") not in cand  # filtered: not the queried provider


def test_parse_boards_dump_groups_by_provider():
    dump = "greenhouse|acme\nashby|clipbook\ngreenhouse|other-co\n"
    out = ats_boards.parse_boards_dump(dump)
    assert out["greenhouse"] == {"acme", "other-co"}
    assert out["ashby"] == {"clipbook"}


def test_parse_boards_dump_ignores_blank_and_malformed_lines():
    dump = "greenhouse|acme\n\nnotaline\nashby|clipbook\n"
    out = ats_boards.parse_boards_dump(dump)
    assert out["greenhouse"] == {"acme"}
    assert out["ashby"] == {"clipbook"}


def test_parse_boards_dump_unknown_provider_lookup_is_empty_not_a_keyerror():
    out = ats_boards.parse_boards_dump("greenhouse|acme\n")
    assert out["workable"] == set()


def test_seed_items_builds_board_company_pairs():
    rows = [("Acme Inc", "acme", 12), ("Beta Co", "beta", 3)]
    assert ats_boards.seed_items(rows) == [
        {"board": "acme", "company": "Acme Inc"},
        {"board": "beta", "company": "Beta Co"},
    ]


def test_merge_seed_items_keeps_existing_entries_not_in_this_run():
    existing = [{"board": "acme", "company": "Acme Inc"}]
    new = [{"board": "beta", "company": "Beta Co"}]
    merged = ats_boards.merge_seed_items(existing, new)
    assert {"board": "acme", "company": "Acme Inc"} in merged
    assert {"board": "beta", "company": "Beta Co"} in merged
    assert len(merged) == 2


def test_merge_seed_items_lets_a_fresher_entry_replace_the_stale_one():
    existing = [{"board": "acme", "company": "Acme (stale name)"}]
    new = [{"board": "acme", "company": "Acme Inc"}]
    assert ats_boards.merge_seed_items(existing, new) == [{"board": "acme", "company": "Acme Inc"}]


def test_merge_seed_items_with_no_existing_file_is_just_the_new_rows():
    assert ats_boards.merge_seed_items([], [{"board": "acme", "company": "Acme Inc"}]) == \
        [{"board": "acme", "company": "Acme Inc"}]


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
