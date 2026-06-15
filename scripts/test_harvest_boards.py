#!/usr/bin/env python3
"""Plain-assert tests for harvest_boards HN harvester (no pytest, stdlib only).
Run: python3 scripts/test_harvest_boards.py"""

import sys
import traceback
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import harvest_boards as h  # noqa: E402


def test_select_hiring_threads_filters_to_whoishiring_titles():
    hits = [
        {"objectID": "1", "title": "Ask HN: Who is hiring? (June 2026)"},
        {"objectID": "2", "title": "Ask HN: Freelancer? Seeking freelancer? (June 2026)"},
        {"objectID": "3", "title": "Ask HN: Who is hiring? (May 2026)"},
    ]
    assert h.select_hiring_threads(hits, months=3) == ["1", "3"]


def test_select_hiring_threads_caps_to_months():
    hits = [{"objectID": str(i), "title": "Ask HN: Who is hiring? (m)"} for i in range(5)]
    assert h.select_hiring_threads(hits, months=2) == ["0", "1"]


def test_hn_company_name_takes_leading_token_and_strips_tags():
    comment = "<p>Acme Corp | Senior Backend | Remote | https://example.com</p>"
    assert h.hn_company_name(comment) == "Acme Corp"


def test_hn_company_name_handles_plain_text():
    assert h.hn_company_name("Globex - we are hiring") == "Globex"


def test_extract_hn_candidates_attributes_leading_company_name():
    comments = [
        '<p>Acme Corp | Backend | Remote</p><p>Apply: '
        '<a href="https://job-boards.greenhouse.io/acme">link</a></p>',
    ]
    cand = h.extract_hn_candidates(comments)
    assert cand[("greenhouse", "acme")] == "Acme Corp"


def test_extract_hn_candidates_unescapes_entities():
    # HN encodes URL slashes as &#x2F; — must unescape before regex.
    comments = ['Foo Inc | jobs at https:&#x2F;&#x2F;jobs.lever.co&#x2F;fooinc']
    cand = h.extract_hn_candidates(comments)
    assert ("lever", "fooinc") in cand


def test_extract_hn_candidates_falls_back_to_slug_when_no_name():
    comments = ['<a href="https://jobs.ashbyhq.com/standalone">x</a>']
    cand = h.extract_hn_candidates(comments)
    # leading token is empty-ish -> name falls back to slug
    assert cand[("ashby", "standalone")] in ("standalone", "x", "Standalone")


def test_hn_company_name_rejects_prose_and_role_lines():
    assert h.hn_company_name("Do you want to work on distributed systems? We build") == ""
    assert h.hn_company_name("Software Engineer -- Infrastructure | Remote") == ""
    assert h.hn_company_name("<p>At SentiLink, we stop identity fraud at scale</p>") == ""
    assert h.hn_company_name("Sheer Health ( https://x.com )") == ""


def test_extract_hn_candidates_uses_title_slug_for_junk_name():
    comments = ['Software Engineer | role | https://job-boards.greenhouse.io/acme-corp']
    cand = h.extract_hn_candidates(comments)
    assert cand[("greenhouse", "acme-corp")] == "Acme Corp"


def test_slug_title_humanizes():
    assert h._slug_title("acme-corp") == "Acme Corp"
    assert h._slug_title("thelabnyc") == "Thelabnyc"


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
