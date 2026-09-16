## Why

freehire#1636 requests onboarding Joppy (joppy.me), a small Barcelona-based, Spain-only tech
hiring platform (~80 open postings across ~37 companies at last check). It is publicly reachable
(`robots.txt` is `Allow: /`, no login required) and, unusually for a board this small, most
postings carry a published salary range, a structured must-have/nice-to-have skill list, required
language levels, and visa/relocation flags — worth mapping rather than just title/company/location.

## What Changes

- Add a new `joppy` source adapter (`internal/ingest/sources/joppy.go`) that crawls the whole
  Joppy company directory each run: it reads `https://www.joppy.me/sitemap.directory.xml` for the
  full list of `/companies/<slug>` URLs, then fetches every company page and reads that company's
  full open-jobs array out of the page's embedded `__NEXT_DATA__` payload (the same
  embedded-JSON-in-a-server-rendered-page technique already used by other Next.js-backed adapters
  in this codebase, e.g. `talenthr`, `alignerr`, `epam`).
- The adapter is boardless (one global `joppy` registry entry, no per-employer `boards` rows) and
  needs no separate per-posting detail request: a company page already carries every one of its
  open postings in full (title, HTML body, structured skills with a must-have flag, place/work-mode
  flags, required language levels, and — when the employer opted to publish it — a salary range).
- Register `joppy` in `sources.All` and mark it a boardless aggregator (company comes from each
  posting, not from the board config), matching the `getmanfred`/`arbeitnow` shape.
- Map the platform's structured signals onto freehire's existing `Job` fields where they fit
  (`WorkMode`, `Skills`, `EnglishLevel`, `SalaryMin`/`SalaryMax`/`SalaryCurrency`/`SalaryPeriod`);
  fields with no dedicated column today (must-have skill flag, visa sponsorship, relocation
  package, EU-candidates-only) are folded into the description text rather than dropped, since
  `Job` has no structured slot for them.

## Capabilities

### New Capabilities
- `joppy-source`: crawls the Joppy company directory (sitemap + per-company `__NEXT_DATA__` page)
  and maps each posting onto freehire's normalized `Job` shape.

### Modified Capabilities
(none — no existing capability's requirements change)

## Impact

- New file `internal/ingest/sources/joppy.go` + `joppy_test.go`.
- One new line in `sources.All` (`internal/ingest/sources/registry.go`) registering the adapter.
- No schema change, no new env var, no new worker — `cmd/ingest joppy` crawls it like any other
  provider once registered. Operationally, after merge, one boardless catalog row still needs
  adding via `cmd/add-board --provider=joppy --apply` (empty board id) so the provider is
  scheduled — the same one-time step every new provider needs, out of scope for this PR's code.
