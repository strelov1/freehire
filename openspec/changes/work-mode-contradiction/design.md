# Design

## The denial dictionary, and why it is not the existing phrase list

`descriptionWorkModePhrases` already holds an `onsite` family (`"on-site only"`,
`"must be onsite"`, `"fully on-site"`, …). Reusing it for the override was the obvious
move and the measurement rejected it.

Over 7 313 `remote` postings sampled from prod, the candidate families fired like this:

| family | fired | genuinely onsite |
|---|---|---|
| `not a remote position/role/job` | 14 | 14 |
| `100% on-site` | 1 | 1 |
| `fully on-site` | 2 | **0** |
| `does not offer remote`, `no remote work` | 0 | — |
| `on-site only`, `must be onsite` | 0 | — |

Both `fully on-site` hits were the same employer writing *"fully on-site **for the first
90 days** at the Edwardsville, IL headquarters. After successful …"* — a temporary
arrangement in a posting that really is remote afterwards. A phrase that reads absolute in
a list turns out to be routinely qualified in prose. `fully on-site` is therefore NOT in
the denial dictionary.

`100% on-site` is in it, but only behind the guard those ADP postings taught: a match is
rejected when a qualifier follows within 60 characters. The qualifier list is written from
observed sentences rather than imagined ones, and covers the two shapes prod actually
carries — a trial period before remote opens up (`for the first`, `after the first`,
`for the initial`, `after the initial`, `after successful`) and a follow-on role that is
not this posting (`if hired`, `once hired`, from a DoD SkillBridge internship whose own
header reads "This is a remote position" and whose body adds "this is not a remote job IF
HIRED afterward"). The guard applies to every denial, not just this one: a qualified
denial anywhere is a denial about something else. The scan then continues rather than
returning, so a posting that hedges one sentence and states another plainly is still read
as denying.

`on-site only` and `must be onsite` are left out. They fired on nothing in 9 813 sampled
postings, so they buy no coverage, and both have an obvious false-positive shape a sample
this size would not necessarily surface — "parking on-site only", "must be onsite for
quarterly planning" in a posting that is otherwise remote. A dictionary entry that cannot
be shown to help and can be argued to hurt does not go in.

What remains is denial by construction — sentences whose only purpose is to say the job is
not remote:

```
100% on-site                     (unless "for the first …" follows)
not a remote position/role/job/opportunity
this position/role/job is not remote
position/role/job is not (a) remote
does not offer (a) remote
no remote work/option/options
remote work is not available / not an option / not offered
```

## Markup is not information

Descriptions are stored HTML, and prod carries both `This is not a <strong>REMOTE
POSITION</strong>` and `This is&nbsp;not a remote position`. Matching the raw string reads
the first as silent: 2 of 15 measured denials were lost to a bold tag alone. So
`RemoteContradicted` folds tags and entities away and collapses whitespace before matching,
while `WorkModeFromDescription` keeps matching raw. The asymmetry is the point — that one
fills a blank, so a phrase it misses costs a facet nobody had; a phrase this one misses
leaves a wrong facet standing.

## The final measurement

The implemented algorithm — the dictionary, the qualifier guard, the markup fold — was then
run against prod as a whole, over three disjoint slices of `remote` postings and two
controls:

| slice | sampled | fired | genuinely onsite |
|---|---|---|---|
| `remote`, newest first | 3 000 | 6 | 6 |
| `remote`, oldest first | 2 000 | 0 | — |
| `remote`, by posted date | 2 000 | 3 | 3 |
| **`remote` total** | **7 000** | **9** | **9** |
| `hybrid` (control) | 1 000 | 0 | — |
| `onsite` (control) | 1 000 | 21 | agreement, not error |

Every one of the nine names a city or states the requirement in the same sentence
(Disney, TE Connectivity, jazzhr, paycor, freshteam, workable, smartrecruiters). The
`onsite` control is the strongest evidence that the dictionary measures the right thing: it
fires on postings already labelled onsite 16 times more often than on postings labelled
remote, which is the direction a correct denial detector must show.

At 0.13% of the 156 626 open remote postings, the change is expected to correct a few
hundred rows — and to correct them on the nightly `backfill-derive`, since the derivation is
shared.

## Where the contradiction sits in the precedence chain

`jobderive.Derive` resolves work mode as structured → location marker → description
phrase. The contradiction is not a fourth tier under those; it is a check ABOVE the whole
chain's result:

```
workMode := <existing three-tier resolution>
if workMode == "remote" && location.RemoteContradicted(in.Description) {
        workMode = "onsite"
}
```

It overrides a structured ATS signal too, and that is deliberate. Every one of the 22
measured hits came through a structured or location-derived `remote`: Workday, jazzhr,
paycor, successfactors, workable, smartrecruiters, freshteam. An ATS "remote" is routinely
a requisition's location bucket rather than a work arrangement — exactly what NVIDIA's
`location = "US, TX, Remote"`, `remoteType = null` is — while a sentence saying the role is
not remote is the employer writing prose about the arrangement itself. The prose is the
better witness, and only for this one narrow question.

The check runs only against `remote`. A `hybrid` result is left alone: "not a remote
position" is a true statement about a hybrid job, and flipping it to `onsite` would replace
a right answer with a wrong one.

## Why a stale body is never re-read today

A hydrating adapter's `FetchNew` is handed a `seen` predicate built from
`dbStore.ExistingExternalIDs`. A posting the catalogue already holds returns a
`SeenRefresh` job (or, for the board-shaped sources, takes `Touch`), which refreshes
`last_seen_at` and writes no content — by design, since a content-less re-upsert would wipe
the description hydrated when the posting was new. Nothing else ever fetches that
posting's detail again.

The seam for changing that already exists. `ExistingExternalIDs` withholds a row from the
set when it has no description and is younger than `hydration_cutoff`, which re-offers it
for detail exactly as if it were new. Staleness is the same idea with a different
predicate.

## `hydrated_at`, and why `updated_at` cannot serve

`persist` tries `RefreshUnchangedJob` first and falls back to `UpsertJob`; the cheap path
deliberately leaves `updated_at` alone, because `updated_at` means "content changed". The
common case for a re-read is that the body is byte-identical, which takes the cheap path —
so a staleness predicate over `updated_at` would leave the row looking stale forever and
re-fetch it on every crawl of its slot. That is the opposite of what the feature is for.

`jobs.hydrated_at timestamptz` is written by BOTH write paths, and means one thing: the
last time this row was written from a body we had just fetched. It is not written by
`TouchJob`, which is the path that fetches nothing. `NULL` — every row that predates the
column — reads as "never checked", i.e. stale, which is correct and needs no backfill; the
slot arithmetic keeps the resulting backlog from arriving at once.

`ADD COLUMN … timestamptz` with no default and no constraint is a catalogue-only change in
PostgreSQL, so the migration does not rewrite the 8M-row table and does not need the
`no-transaction` marker.

## Spreading the re-reads

Withholding every stale row at once would face a hydrating provider with one detail request
per stored posting — 1.27M on workday. The predicate therefore withholds a stale row only
when it falls in the run's slot:

```
abs(hashtext(external_id)) % @refresh_slices = @refresh_slot
```

`hashtext` is deterministic within a database, so a posting belongs to one slot for its
whole life, and a run touching slot *k* re-reads roughly `1/slices` of the stale rows.
`refresh_slot` is the day-of-year modulo `refresh_slices`, so a full sweep of the catalogue
takes `slices` days. A row re-read early in a day gets `hydrated_at = now()` from the write
and drops out of the predicate for the rest of that day's crawls, which is what keeps the
repeated crawls of one board from paying for the same slot again and again.

`BODY_REFRESH_DAYS` unset disables the arm outright, and that is the shipped default: the
predicate collapses to today's behaviour and no extra request is made. Disabling travels
through the same parameters as enabling — `slot = -1`, which `abs(…) % slices` can never
equal — so there is one predicate to reason about rather than two code paths that could
disagree. A set-but-unparseable value fails the run naming the value, the contract its
neighbour `hydrationRetryWindowFor` already has, and a `BODY_REFRESH_SLICE` set without
`BODY_REFRESH_DAYS` fails too rather than quietly doing nothing. `BODY_REFRESH_SLICE`
defaults to 30.

The three seen-set knobs (`hydrationWindow`, `refetchAll`, `bodies`) move into one
`seenPolicy`: they are one decision made in one place, and threading the third one
positionally would have given `newDBStore` seven arguments.

## Testing

- `internal/dict/location/workmode_test.go` — table over every denial phrase, the
  `for the first` guard (both ADP sentences, verbatim), and the NVIDIA sentence; plus the
  negative cases that must NOT fire: "remote team", "hybrid cloud", "parking on-site only".
- `internal/job/jobderive/jobderive_test.go` — a structured `remote` overridden to
  `onsite`; a location-derived `remote` overridden; a `hybrid` left alone; a `remote` with
  no denial left alone.
- `cmd/ingest` — the slot/staleness predicate is SQL, so its unit-level cover is the knob
  parsing (`bodyRefreshWindowFor`) and the disabled-by-default case; the predicate itself is
  covered by the existing `internal/platform/db` integration tests, extended with a row
  whose `hydrated_at` is old and one whose is fresh.
