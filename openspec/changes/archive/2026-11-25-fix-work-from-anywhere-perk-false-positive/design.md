## Context

See proposal.md for motivation. `internal/dict/location/workmode.go` already ships
one guard of this exact shape — `remoteDenialPhrases` / `denialQualifiers` /
`denialQualifierWindow`, used by `RemoteContradicted` — for the opposite direction
of the same problem: a phrase that is usually unambiguous but is qualified in
specific, measured real-world sentences. That guard only scans forward from the
match (`RemoteContradicted`'s loop advances `rest` past the match and checks the
next `denialQualifierWindow` characters). Production evidence for this change
(freehire#2696 investigation, read against the live Meilisearch `jobs` index) shows
the qualifier for "work from anywhere" appears on **either side** of the match:

- After: "the freedom to **work from anywhere** in the world **for up to** a
  month", "**work from anywhere** within the EU **for up to** 140 days a year"
- Before: "benefits include **up to** 12 days **work from anywhere**", "a Work from
  Anywhere policy allowing **up to** 20 remote days"

So the new guard needs a bidirectional window, not a reuse of the existing
forward-only scan.

`WorkModeFromDescription` is a flat priority loop over
`descriptionWorkModePhrases` (hybrid family, then remote family, then onsite
family; first phrase match anywhere in any family wins). The two phrases needing
the guard ("work from anywhere", "work-from-anywhere") sit inside the `remote`
family's phrase list alongside ten others that measurement showed do **not** need
it (see proposal.md's Impact / the "not comparable" note below).

## Goals / Non-Goals

**Goals:**
- Stop "work from anywhere" from filling `work_mode=remote` when it is describing a
  bounded travel/PTO allowance rather than the posting's actual arrangement.
- Keep the fix scoped to exactly the phrase(s) measurement showed are ambiguous —
  match the file's existing "measured evidence, not symmetry" doctrine
  (`remoteDenialPhrases`'s own comment explains why `fully on-site`/`on-site only`
  were tried and rejected on the same basis).

**Non-Goals:**
- Detecting hybrid from phrasings like "hybrid culture" or "in person 3 to 4 days a
  week" that the current `hybrid` phrase family misses. Investigated during triage;
  sampling production data found real false-positive risk in the most obvious
  candidate addition ("hybrid culture" co-occurs with genuinely-remote postings that
  merely mention a hybrid company culture in unrelated prose). Per this package's
  "never guess" doctrine, once this change stops the wrong `remote` fill, a posting
  with no other matching phrase correctly lands on an empty `work_mode` — that is
  the correct output, not a gap this change needs to close.
- Widening the guard to any other remote phrase. Checked and rejected — see
  Decisions.
- Any change to `jobderive.go`'s precedence chain or to `RemoteContradicted`. Both
  are correct as-is; this change only narrows what `WorkModeFromDescription` fills.

## Decisions

**Bidirectional window, not a forward-only reuse of `denialQualifierWindow`/
`containsAny`.** The existing denial-qualifier scan only needed to look forward
because every observed denial qualifier ("for the first 90 days", "if hired")
follows the denial phrase it qualifies. Real "work from anywhere" postings write
the qualifier on either side, so the new check inspects a window before **and**
after the match. Window size: reuse `denialQualifierWindow`'s value (60 chars) —
the observed distances in production sentences are well inside it (worst case
observed: "the freedom to **work from anywhere** in the world **for up to** a
month", ~24 chars from the end of the phrase to the start of the qualifier), and
reusing the constant avoids inventing an unmeasured second number.

**Qualifier marker: "up to" alone**, not a longer enumerated list like
`denialQualifiers`. Evidence: every disqualifying production sentence found during
triage carries "up to" (`for up to`, `up to N days/weeks/a month`, `allowing up to
N remote days`), and "up to" is inherently a bounded-quantity marker, unlike the
denial qualifiers (`for the first`, `if hired`, ...) which needed to be enumerated
because they describe unrelated scoping shapes (a trial period vs. a follow-on
role). Cost of a false suppression here is low by design (see Risks) which affords
a broader, simpler marker than the denial side needed.

**Scope the guard to exactly `"work from anywhere"` and `"work-from-anywhere"`, not
the whole `remote` family.** Checked co-occurrence of the other remote phrases
(`100% remote`, `fully remote`, `remote-first`, `remote role`, `remote position`,
etc.) with "for up to" in production: all far higher-volume (272–1,721 hits vs. 143
for "work from anywhere") but inspected samples showed this is unrelated noise (a
salary range, an unrelated benefit figure), not the same ambiguity — none of those
phrases double as a common bounded-perk idiom the way "work from anywhere" does
("100% remote for up to X days" is not an idiom employers write). Guarding them
would cost real coverage (a "100% remote" posting mentioning an unrelated "up to"
figure would wrongly lose its remote fill) for no measured benefit — the same
reasoning `remoteDenialPhrases`'s comment already applies to `on-site only`/`must
be onsite`.

**When suppressed, keep scanning for the SAME phrase before moving to the next
one.** Falling through to the next phrase in `descriptionWorkModePhrases` handles a
description with both a qualified "work from anywhere" and an unrelated,
unqualified remote/hybrid/onsite phrase elsewhere — but it is not enough on its
own: a description can also state "work from anywhere" TWICE, once qualified
("up to 12 days work from anywhere per year") and once plainly ("you can work from
anywhere in the EU"). A single `strings.Index` per phrase only ever sees the first
occurrence, so a naive "skip and fall through to the next phrase" implementation
would miss the second, unqualified occurrence of the *same* phrase entirely and
return `""` where `"remote"` is correct. `phraseMatches` therefore re-scans forward
past a suppressed match for more occurrences of the same phrase — the identical
"scan past a qualified match" property `RemoteContradicted`'s loop already has for
denial phrases (and the same one `TestRemoteContradictedReadsPastAQualifiedDenial`
guards there). Caught in review (not in the original design) and closed with a
matching test case before merge.

## Risks / Trade-offs

- **[Risk]** A genuinely fully-remote posting that also happens to mention an
  unrelated "up to" figure within 60 characters of "work from anywhere" (e.g. "work
  from anywhere, with a signing bonus of up to $2,000") loses its remote fill from
  this phrase. → **Mitigation**: the cost is an empty `work_mode`, not a wrong one —
  this package's `WorkModeFromDescription` doc comment already states that a phrase
  it misses "costs a facet nobody had", the accepted trade-off for the whole
  gap-filling list (unlike `RemoteContradicted`, which is held to a stricter bar
  because it overrides a value already believed correct). A posting in this shape
  is also likely to carry a structured or location-based remote signal already
  (most `remote`-labeled "work from anywhere" hits in the production sample were
  resolved by a higher-priority source, never reaching this fallback at all), so the
  practical blast radius is small. Not testing for this edge case within
  `workmode_test.go` beyond the general shape already covered (see tasks.md) —
  no production evidence of it occurring was found during triage.
- **[Trade-off]** The guard does not attempt to detect hybrid in these postings, so
  a posting like the reported one (hybrid, `work_mode` currently wrongly `remote`)
  moves to `work_mode=""` rather than the correct `hybrid`, until the hybrid phrase
  family separately gains coverage (out of scope — see Non-Goals). → Accepted: empty
  is strictly better than wrong for every downstream consumer (search facet,
  `jobview`), and matches the project's stated dictionary doctrine.

## Migration Plan

Code-only change, no schema/migration. Deploys through the normal release path.
After deploy, existing rows carrying the wrong `work_mode=remote` from this phrase
need `cmd/backfill-derive` (re-derives all six dictionary facets in one pass) and
then a full `cmd/reindex` to reach Meilisearch — the standard two-step propagation
this package's `AGENTS.md` documents for any dictionary change. Not part of this
code change's tasks; call out as a deploy follow-up in the PR description.
Rollback: revert the code change; no data migration to undo (the backfill only ever
narrows a previously-wrong `remote` toward empty, which is a strict improvement
even if left in place after a revert).
