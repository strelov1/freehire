## Why

`internal/atsboard` states a contract in its own package doc: *"One definition means a host added
once is recognised by all three."* `internal/atsdetect.FromURL` was a second, independent table
answering the same question — which board does this URL address — for ~16 providers, eleven of
them already in `atsboard`.

The finding framed `atsdetect` as the drifted copy. **The drift runs both ways, and the shared
table is the one that costs money.** `atsboard` is the accept-set for `internal/contribution`,
which pays for onboarded boards, and its own doc names the failure mode: a board recorded as new
"is paid for". Comparing the two implementations on the same URLs found five divergences —
**four of them `atsboard`'s**, none covered by a test:

| URL shape | atsboard | atsdetect |
|---|---|---|
| `…/en-us/Careers/job/…` (lowercase locale) | board `…/en-us` ✗ | `…/Careers` ✓ |
| `…/job/Berlin/Eng_R-1` (no career site) | board `…/job` ✗ | declined ✓ |
| `…/details/Eng_R-1` | board `…/details` ✗ | declined ✓ |
| `apply.workable.com/j/EF5014296F/apply` | board `j` ✗ | declined ✓ |
| `careers.pageuppeople.com/cw/en/search` | board `cw` ✗ | declined ✓ |
| `uk-ext.eu.csod.com/…` | board `uk-ext` ✗ | declined ✓ |
| `jobs.smartrecruiters.com/Portal/Acme/…` | `Acme` ✓ | board `Portal` ✗ |

Every ✗ on the `atsboard` side names a board that does not exist — precisely what the paying flow
must not do.

## What Changes

- **`atsboard` gains the six narrowings `atsdetect` already knew**, each with the test it never
  had: the locale match accepts a lowercase country; a Workday path leading with `job`/`details`
  carries no site; a `noBoardFirstSegments` hook declines Workable's `/j/<id>` shortlink outright
  (skipping `j` would take the JOB id as the board, which is worse); `modePathNumeric` requires
  PageUp's numeric institution id; and `subdomainLabel` declines a multi-label remainder, because
  those adapters crawl `<board>.<apex>` and `uk-ext.eu.csod.com` has no such form.
- **`FromURL` delegates to `atsboard.Recognize`** and keeps only the five shapes `atsboard`
  deliberately excludes (icims, oracle, taleo, neogov, paycom). Eleven overlapping cases and two
  now-unused helpers are deleted.
- **A test keeps the two sets disjoint.** If a local shape starts being recognised by the shared
  table, either this package's case is dead code shadowed by the delegation, or the paying
  accept-set widened without anyone arguing for it — and the failure says to decide which rather
  than delete one.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) at the requirement level — no endpoint or contract changes. The behaviour that changes is
six URL shapes that previously resolved to non-existent boards and now resolve to none.
`tasks.md` is the real artifact; the change archives with `--skip-specs`.

## Impact

- `internal/atsboard` (+ tests), `internal/atsdetect` (+ tests).
- Consumers of `atsboard.Recognize` — `internal/contribution`, link resolution, `boardresolve` —
  stop being offered six shapes' worth of false boards. Nothing that previously resolved to a
  REAL board changes.
- `cmd/harvest-role`, the one production caller of `FromURL`, widens from ~16 providers to
  `atsboard`'s ~46. That is the standing cost the split imposed, and it is safe: harvest writes
  per-provider seed files that `cmd/harvest-boards` probes against each platform's API before
  committing, and it is invoked per provider by hand.
- **Deliberately NOT done:** moving the five local shapes into `atsboard`. Widening the accept-set
  for a flow that pays is its own proposal.
