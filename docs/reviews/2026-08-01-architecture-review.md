# Architecture review — 2026-08-01

A whole-repository review of `freehire` along four axes: **code reuse**, **abstraction
boundaries**, **coupling**, and **simplicity**. Nine reviewers each took one slice of the
codebase and applied all four lenses; a skeptic then re-checked every finding against the code
with instructions to refute by default. 46 findings survived; the refuted ones are not recorded
here.

Every finding carries `file:line` evidence that was opened during the review. The `Verifier`
block under each finding is the skeptic's independent re-check — it frequently narrows, corrects
or downgrades the finding above it, and is often more useful than the finding itself.

## Progress

Status markers are kept on the ranked shortlist table and on each `S*` heading, so this file
doubles as the burn-down. Legend: ⬜ not started · 🔄 in progress · ✅ merged to `main`.

| Status | Findings |
|---|---|
| ✅ **closed** | The shortlist (S1–S20) and every actionable catalogue finding are done. **Nine remain, and all nine are deliberate `leave as is`** — their verifier notes reject the remedy as a premature generic or point at a documented decision (#3, #5, #6, #10, #23, #35 and neighbours). Do not re-open one by its number: read its `Verifier` block first. #20 was found already closed by the S20 work and needed no PR. |
| ✅ merged | S1 (#1393) · S2 (#1395) · S3 (#1394) — released and archived in #1398
            S4 (#1385) · S5 (#1389) · S6 (#1390) — archived in #1391
            S7 (#1403) — archived in #1404
            S8 (#1405) — archived in #1406
            S9 (#1409) — archived in #1412
            S10 (#1414) — archived in #1415
            S11 (#1416) — archived in #1417
            S12 (#1420) — archived in #1422
            S13 (#1426) — archived in #1428
            S14 (#1430) — archived in #1431
            S15 (#1433) — archived in #1434
            S16 (#1435) — archived in #1436
            S17 (#1438) — archived in #1439
            S18 (#1444) — archived in #1447
            S19 (#1448) — archived in #1450
            S20 (#1452) — archived in #1453 — **the ranked shortlist is complete**
            catalogue #19+#34+#45 (#1456) — archived in #1459
            catalogue #27+#28+#29 (#1462) — archived in #1463
            catalogue #36+#38 (#1465) · #16+#32 (#1470)
            I1 (#1474) — archived in #1475
            catalogue #20 — **found already closed**, no PR
            catalogue #2 (#1478) — archived in #1481
            catalogue #24 + #15 + #9 (#1487) · #4 (#1492) — refactors, no spec delta
            catalogue #39 + #40 (#1494) — the residues only; both verifiers' rejections upheld
            catalogue #11 (#1497) — prose, not an extraction; verifier's rejection upheld |

**S1 · S2 · S3** — merged, released to freehire.me (blue/green, zero migrations) and verified
live before archiving in #1398.

- **S1** — `unify-facet-display-labels`, #1393. `CATEGORY_LABELS` is exhaustive over the
  generated vocabulary and constrained by `satisfies Record<Category, string>`, so an unlabelled
  new category (TS1360) and a stale leftover label (TS2353) each fail `pnpm run check`. The
  `/insights` fork is gone. Review found a **fourth** surface the finding missed —
  `routes/open/+page.svelte` printed "C level" and "Onsite" on a sitemap'd page. Verified on
  prod: `/open` reads C-level / On-site with zero occurrences of the old spellings.
- **S2** — `cluster-geography-incremental-push`, #1395. The finding understated the cause:
  because the push is a *field-level* update and the geography facets carry no `omitempty`, the
  three writers were not merely failing to widen — they were **overwriting** the reindex's union.
  New per-cluster `RoleClusterGeo`; ingest, link import and embed now widen the canon. Review
  caught a destructive default: a failed cluster-count also skipped the merge, reinstating the
  bug. Verified on prod against the live catalogue.
- **S3** — `backfill-workers-shared-bootstrap`, #1394. Both backfills on `worker.Main` +
  `worker.Bootstrap`; every `log.Fatal`/`os.Exit` is a return value. The rule is now a test that
  derives its population from behaviour rather than a hand-kept list. Review caught a regression
  the fix itself introduced: the newly signal-bound context turned one SIGTERM into a fleet of
  per-user failures because neither loop checked `ctx.Err()`.

**Ops findings raised while releasing S3** (neither caused by the change, neither fixable by a PR
in this repo): `release.sh` does not build `backfill-experience` or `backfill-resume-structured`
although every other `backfill-*` one-off is in its list, so a repair run still needs a hand
build; and `PII_FILTER_URL` is set nowhere on the host with `pii-filter` inactive, so
`backfill-resume-structured` refuses to start in production by design.

**S4** — `job-aggregate-derived-columns`, merged in #1385. The aggregate's write mapping now owns
both derived columns; `internal/jobhash` is imported by no write path, and the aggregate is
the only place either manual params struct is built. Wider than the finding stated:
`cmd/tg-extract` and `internal/linkimport` were also writing `content_hash` NULL, so the
upsert's `changed` signal was dead for both sources.

**S5** — `evidence-gate-by-construction`, merged in #1389. The gate is a constructor argument;
`Editor.WithEvidenceGate` and `bankGate`'s nil-bank branch are gone. The review called this
latent — a new integration test assembling the CV surface with no assistant showed it was
reachable: an API-key `PATCH` inserting an uncited bullet returned **200** and the claim
landed. It is 403 now. Raised finding I1 below.

**I1** — `fixtures-obey-the-evidence-gate`, merged in #1474. The product question it demanded be
answered first was **already answered in the code**: `Register` passes `bankGate{bank}`
unconditionally, so aligning the fixtures changed no product behaviour. The other reading — a
per-actor policy — would be a product change and stays a separate proposal rather than something
smuggled in through a test fix.

Running the fixtures against production's real configuration surfaced three orderings the nil
gate had hidden, and they are worth knowing:

- **The gate is the OUTER check.** An uncited malformed op is 403, not the 422 the addressing
  check gives.
- **It also runs before ownership.** A foreign caller sending a claim-bearing op never reaches the
  404. Isolation holds — 403 is what an uncited claim gets whether or not the CV exists — but only
  a non-claiming op exercises the 404.
- **Eight more fixtures had nil gates** that I1 does not name. Inert today (none edit as the
  agent), but an agent-path case added later would have lost the wall silently.

A rule test now refuses a nil gate anywhere in the package, verified by restoring one. That is the
general lesson from I1: **a fixture that bypasses the constructor is immune to the fix that hardens
it** — S5 made the gate a construction-time argument and these three neither broke nor improved,
because they never called the constructor.

**Catalogue #16 + #32** — `docs-describe-the-real-mechanism`, merged in #1470. Both overstated,
and both the same shape underneath: **a document describing a mechanism the code does not have.**
That is now SEVEN of the findings closed so far whose real defect was a true-sounding sentence
nothing enforced — it is the single most common failure in this codebase, well ahead of
duplication.

Two methods worth reusing on the remaining eighteen:

- **Read past the grep the finding cites.** `CLAUDE.md` claimed two workers "hold their own
  flock"; the finding proved no flock in Go. Reading the actual systemd units on production
  showed what DOES serialize them — `Type=oneshot` refusing a second instance — and therefore
  that a **hand-started run has no lock at all**, which is the real basis of the "never stack"
  warning. Deleting the sentence would have lost that.
- **Measure the blast radius instead of arguing about it.** #16 reads like data loss (a backfill
  blanking moderator-stated geography). On production: 7 manual rows, all 7 still carrying
  regions. That turns a debate into a decision to write down — and the note records that
  `internal/moderation`'s edit path already does the same thing on a title typo fix, so the fix,
  if the intent ever changes, goes in BOTH doors or neither.

**Catalogue #36 + #38** — `one-slug-set`, merged in #1465. `SlugSet` collapses the three
job-slug stores; each is five lines now.

**#36 turned out to be already closed by S1** — one `titleCase`, one `categoryLabel`, one
`RELOCATION_LABELS` with a test. I nearly re-did merged work. The lesson for whoever picks up the
rest: **the catalogue's `#N` numbering and the shortlist's reviewer ids do NOT line up** (S1's id
is `38+42`, which are not catalogue #38 and #42), so overlap must be checked in the CODE before
starting, not inferred from titles or ids.

Also worth carrying: no unit test was possible for `SlugSet`, and the reason is written into the
repo already — `paginated.svelte.test.ts` records that `$state` fields need a Svelte runtime this
test env does not provide. Saying that in the PR is better than a test that instantiates nothing.

**Not attempted, deliberately: #40 (six tab-strip implementations vs the unused `Tabs` primitive).**
Swapping six visible components with no way to render them in this session is how a "no visual
change" claim becomes false. It wants eyes on it, or a session with the app running.

**Catalogue #27 + #28 + #29** — `mail-and-silence-one-rule-each`, merged in #1462. All three
carried the verdict **overstated**, and checking that before acting changed the work in a way
worth recording, because it produced a THIRD outcome beyond S17's and S18's:

- **#29's live claim was already refuted by its own verifier**, and I confirmed the reasoning
  rather than inheriting it (`GREATEST` ignores NULLs, so `last_activity_at` is NULL exactly when
  `applied_at` is). What survived was three copies of a four-line day count.
- **#27's live claim I could not construct at all.** It needs a message carrying a live suggestion
  AND a set `job_id`; every writer either clears the suggestion when it links or attaches the
  application alongside it. The predicate is still aligned — two spellings of one rule held apart
  by a coincidence of writer behaviour is not an invariant — but the SQL comment now says it
  closes a drift risk rather than fixing a bug.

So "compare before you consolidate" has now produced all three answers: the finding was backwards
(S17), the finding was right (S18), and the finding's mechanism is real but its exploitability is
not demonstrated (#27/#29). The third is the one that most needs saying out loud, because a PR
that repeats an unverified live claim teaches the next reader to trust the next one.

#28's remedy was deliberately modest and stayed that way: `notify.ValidChannel` exported, two
hand-built allowlists deleted, and `notifications.md` corrected — it claimed adding a channel
"means adding a package, not touching `notify` or `reminder`", which is false. Collapsing the
duplicated `Router`/`Notifier` pair is deferred until a third channel lands, and that deferral is
now written down as a decision.

**Catalogue #19 + #34 + #45** — `one-llm-config-shape`, merged in #1456. Three independent
reviewers filed the same duplication, which is why it was worth taking first out of the
catalogue: `config.Settings` and `config.Enrich` read the same six env keys, and seven
entrypoints hand-copied them into `llm.Settings`.

**The verifier's correction was the load-bearing part, and it changed the design.** It said
`llm.Settings` is NOT a third copy — it is the env-free shape the library takes so `internal/llm`
depends on no configuration package — and that the two config structs differ in POLICY on
purpose. So the hoist keeps the values shared and the policy the caller's: the server never calls
`Require` because degrading is its documented behaviour, and a worker whose job is to spend LLM
calls does. A naive "one config struct" would have flattened a deliberate difference.

The real bug the duplication hid was the coupling: `cmd/tg-extract` and `cmd/classify-mail` loaded
the ENRICHMENT config purely for the six LLM values, inheriting `ENRICH_*` validation they have no
use for. Net −93 lines.

**S20** — `classify-mail-reports-its-outcome`, merged in #1452. **The ranked shortlist is done.**

The exit-code rule was already written down: `worker-lifecycle` requires a non-zero exit on any
failure or dead-letter, and eight binaries obeyed it. `classify-mail` could not — the signal was
dropped three layers down, starting at a query that was `:exec` where the two siblings its own
comment claimed to mirror are `:one ... RETURNING attempts, failed_at`. **A missing RETURNING
clause was the whole reason a mail queue could dead-letter every message and exit 0.**

Two things worth carrying into the full catalogue. First, the one spec delta of this change was
not the exit-code requirement — that one was right and simply disobeyed — but *"Bookkeeping
failures are logged and counted"*, which said "the **enrichment** drain MUST…". A correct rule
scoped to the first place it was needed is how the second place drifts. Worth grepping the specs
for other requirements written against one caller.

Second, the case that needed thinking rather than porting: when the bookkeeping write ITSELF
fails, the drain does not know whether the item dead-lettered — and must not guess. It counts as
failed, not dead-lettered, because the entry is then governed by its lease expiry rather than by
a stamp the run never wrote.

**S19** — `drop-the-dead-profile-composer`, merged in #1448. `ProfessionalFrom` deleted; its
sixteen lines of rationale moved onto `Store.Professional`, the path that actually runs them.

Two judgement calls worth carrying forward. **Deleting a function must not delete its reasoning**
— the comment explaining why the structure's own experience is ignored rather than used as a
fallback is the part a future reader needs, and it was attached to the dead twin. And the seven
tests went to `Store.Professional` rather than to `experienceFromBank`, which the remedy offered
as the easier target: asserting the rules through the store reads the fit chain actually makes is
strictly more coverage than they had, and a fake-repo helper already existed.

Sixth of nineteen where the real defect was a doc pointing somewhere false —
`resumeextract/AGENTS.md` answered "what does the fit chain send?" with the name of the function
nothing called.

**S18** — `one-ledger-reconcile`, merged in #1444. `inbox.ReconcileMailEvent` holds the
retract-before-insert rule; `cmd/classify-mail`'s copy is gone.

S17's lesson was applied and gave the opposite answer, which is the useful part: these two really
WERE identical — same steps, same order, same parameters — and the only genuine difference was
error semantics, which the extraction preserves rather than unifies. "Compare before you
consolidate" is not a rule that always finds a hidden defect; it is a rule that tells you which
kind of change you are making.

What neither copy had was a test for the ordering — and the second copy lived in a `cmd/` main,
outside every domain package's test surface, which is why nothing caught that. Three tests now,
the ordering one verified by swapping the statements.

Two docs corrected: `mail-stack.md` said "one reconcile with five callers" (two implementations,
six callers), and `internal/appevent` named `internal/maillink` as a recording path — `maillink`
does not reference `appevent` at all. That is the fifth finding out of eighteen whose real defect
was a true-sounding sentence nothing enforced.

**S17** — `one-url-to-board-definition`, merged in #1438. **This finding's framing was backwards,
and that is its most useful lesson.** It named `atsdetect` as the drifted copy of `atsboard`.
Running both over the same URLs rather than reading them showed the drift runs BOTH ways — and
four of the five divergences were `atsboard`'s, none covered by a test:

| URL shape | atsboard | atsdetect |
|---|---|---|
| `…/en-us/Careers/job/…` lowercase locale | board `…/en-us` ✗ | `…/Careers` ✓ |
| `…/job/…` and `…/details/…` (no site) | board `…/job` ✗ | declined ✓ |
| `apply.workable.com/j/<id>/apply` | board `j` ✗ | declined ✓ |
| `careers.pageuppeople.com/cw/en/search` | board `cw` ✗ | declined ✓ |
| `uk-ext.eu.csod.com/…` | board `uk-ext` ✗ | declined ✓ |
| `jobs.smartrecruiters.com/Portal/Acme/…` | `Acme` ✓ | board `Portal` ✗ |

Every ✗ on the `atsboard` side names a board that does not exist — and `atsboard` is the
accept-set for `internal/contribution`, which **pays**. Its own doc names that failure: a board
"is recorded as a fresh contribution, and is paid for". `atsboard` had NO test for workable,
pageup or cornerstone at all.

**The method is the transferable part: before consolidating two implementations, run both over
the same inputs and find out which one is right.** The review had it the other way round, and
deleting the "copy" on that reading would have kept four live defects in the paying path and
called it a cleanup. Worth applying to S18, which is the same shape (one rule, two
implementations).

The five shapes `atsboard` excludes stayed out, with a test keeping the sets disjoint in both
directions — widening a paying accept-set is its own proposal.

**S16** — `userjob-owns-the-pipeline-rule`, merged in #1435. `userjob.IsTerminal` / `Forward` own
rank and terminality; `mailclassify.AdvanceStage` is signal→stage plus `Forward`. `signalStage`
stayed put on purpose — what an employer's email MEANS is genuinely mail's question, and the seam
runs between meaning and movement rather than between packages.

The finding's drift scenario was reproduced exactly rather than trusted: inserting `take_home`
into `Stages` fails **two** of the three new guards by name. The third is the one worth stealing —
`Aggregate`'s stage→bucket mapping is a `switch`, so nothing can introspect it, but its SHAPE is
checkable: exactly one stage is meant to fall through to the default, so a stage added without a
case shows up as a second one landing there. A behavioural assertion where a structural one is
impossible.

Also worth noting: `Forward` refuses a target that is not an active stage, so advancing INTO a
terminal outcome is now impossible by construction. Before, it was prevented only by the accident
that terminals rank 0 — correct, but for a reason nothing stated.

**S15** — `pgerr-owns-sqlstate-classification`, merged in #1433. `pgerr.IsDataCorrupted` joins its
siblings; `worker.IsCorruptedRow` is gone and `resilient.go` imports no `pgconn` at all.
`internal/enrich` and `internal/embed` stop pulling the bootstrap tree in to classify one
SQLSTATE.

Only the RECOGNITION moved. Deciding that a corrupted row is skipped rather than surfaced is the
resilient scan's policy and stayed in `worker` — the distinction worth keeping when the remaining
findings tempt a "move it all to the shared package".

The pattern that keeps paying: **a package doc that claims a rule is a test waiting to be
written.** `pgerr` said "single home" and was wrong; `labels.ts` said "keep ONE map" and was wrong
(S1); `JobDescription.svelte` said "one home for the CSS" and was wrong (S10); `resumeextract`
argued against blacklists while one ran (S8). Four of the fifteen closed so far were a true
sentence in a comment that nothing enforced. Worth reading the remaining findings for the same
shape.

**S14** — `jobhash-owns-the-row-mapping`, merged in #1430. Both `hashParams` copies deleted;
`jobhash.OfRow` puts the mapping beside the hash it feeds, which is the only place a reader sees
both halves of one decision at once.

The guard is the reusable part: `TestOfRow_CarriesEveryFieldTheHashReads` mutates one hashed field
at a time on a fully-populated row and asserts the fingerprint moves — a dropped field cannot move
it. Proved it fires by removing `Seniority` rather than assuming, which is the discipline worth
applying to every "a test now enforces this" claim on the rest of this list.

**Left open on purpose, and it is the more interesting half of the finding:** `Of` omits `Cities`,
`EnglishLevel` and `IsTech`, which the search document DOES carry — so the change signal
under-covers the document today. Closing it invalidates every stored hash: every job reports
`changed` once on its next crawl and forces a full re-push. That is an operational decision with a
real cost, not a cleanup to fold into a refactor, and it wants scheduling next to a reindex rather
than a PR.

**S13** — `one-sse-stream-writer`, merged in #1426. `streamTurn` rewritten over `sseStream`;
`writeEvent`/`writeComment`, the inline write closure, the bare mutex and the ticker goroutine
deleted, ~50 lines down to ~20. `event` now reports whether the frame reached the client, with a
marshal failure reporting TRUE so a caller that cancels on false cannot be made to cancel by our
own encoding bug.

**I broke the build doing it, and the way I broke it is the transferable part.** Extracting the
four header lines into `sseHeaders` and then replacing them file-wide hit the new function's own
body, which consists of exactly those lines — so `sseHeaders` called itself. The compiler was
silent, `go vet` was silent, and `go test ./...` was green: the recursion only surfaces in a test
that actually drives the endpoint, and those are behind `-tags=integration`, which CI runs as a
SECOND pass I had not run. Docker was available the whole time. Two rules out of it: run both
passes before pushing, and when extracting a body into a helper, never do the call-site
replacement with a whole-file substitution.

Worth carrying into the remaining findings: the keepalive interval is a parameter rather than the
shared constant purely so "stop blocks until the last beat" can be tested at a millisecond
instead of the fifteen seconds production waits. A parameter that exists for testability is not
ceremony.

**S12** — `handler-uses-shared-pgconv`, merged in #1420. `tsFromPtr` and `int4Ptr` deleted; a
**third** twin the finding does not name went with them — `api_keys.go` wrote the same conversion
out inline as a `var` plus an `if`.

One of its evidence lines had already gone stale: *"grep -n pgconv internal/handler/*.go returns
nothing"* stopped being true in #1409, where `pgText` turned out to duplicate `pgconv.Text` and
the handler gained its first import. The same habit surfaced in two findings three apart, which
is the more useful reading of S12 than its "low" severity.

Six `pgtype` literals stay and are NOT missed twins: four are `pgtype.Timestamp` (pgconv has
`Timestamptz`, a different type), one is `pgtype.UUID`, one takes a non-pointer so there is no
nil↔NULL question. Recorded so the next reader does not "finish" the job by inventing converters
nothing needs.

**S11** — `clamp-copies-pagination`, merged in #1416, **confirmed fixed on production**: the same request that answered 500 now answers `200 {"data":[],"meta":{"total":0}}`. The finding was right and understated only
in one way: it reads as a latent overflow, and it was **live**. Confirmed against production
before writing any code — `?offset=3000000000` answered **500**, `?offset=0` answered 200, on a
public unauthenticated endpoint.

The severity label ("low") is the thing to distrust here. Anyone could 500 a public URL; what
made it low was blast radius, not reachability. Worth re-reading the remaining low-severity rows
with that in mind.

`pageParamsMax(c, ceiling)` became `pageParamsBounded(c, fallback, ceiling)` so a caller with its
own default can use it instead of hand-rolling. And the rule is a test now rather than a comment:
the helper's doc comment already named this exact overflow, and naming it did not stop a second
call site re-implementing the parse without the clamp. The test scans the package for any file
reading the offset param outside the helper, derives its population from behaviour, and refuses
to pass if the scan found nothing.

**S10** — `jobdrawer-uses-shared-modules`, merged in #1414. `<JobDescription>` replaces the
inline copy and the 39 duplicated CSS lines are gone; the scroll lock goes through `scrollLock`.
The finding was accurate on both counts, and its line numbers had drifted — worth expecting for
the remaining web findings, since the file moved ~50 lines since the review.

Its "not a live defect" call on the scroll lock is right, and worth stating more precisely than
it did: the drawer is `fixed inset-0 z-50` so it covers the header, and the only other lock
holders are `HeaderMenu`/`HeaderSearch`. Had they been able to overlap, closing the menu would
have set `overflow: ''` under an open drawer. The exemption rested on a layout fact rather than
on the lock's contract, which is why the fix is worth making even though nothing was broken.

Honest gap: verified structurally (byte-identical CSS, identical wrapper classes, same
`:global()` construct, component already live on two other surfaces), NOT by rendering the
authenticated drawer — that needs a signed-in account with tracked jobs and the full stack.

**S9** — `companies-list-wire-type`, merged in #1409. `companyListItem` is the endpoint's wire
shape; `companyRowFromDoc` and `pgText` are gone. The finding's remedy was right down to the
`*string` choice, and it named one thing it did not know: **`pgText` was a duplicate of
`pgconv.Text`**, and `pgconv` had no null-preserving read at all — `TextString` collapses NULL to
`""` — although `TimePtr` and `IntPtr` are exactly that shape. `pgconv.TextPtr` went there rather
than becoming a fourth local unwrapper, so `internal/handler` now imports `pgconv` for the first
time, which is the thing S12 asks for.

The method worth reusing on the rest of the list: for a change that must not move the wire, pin
the contract as **bytes captured from the OLD type**, green before the refactor. A struct
comparison cannot tell a null tagline from an empty one, and field order is part of what a caller
receives — so a value-level assertion would have passed through an API break.

**S8** — `deidentified-cv-typed-seam`, merged in #1405. `atscheck.Analyze` and
`matchanalysis.Input.StructuredResume` take `resumeextract.Professional`; `stripContacts` is
deleted and `candidateContext` collapses to marshal + truncate. The finding was right about the
mechanism and understated the reach: there is a **fourth** model-bound CV path it does not name —
`buildHardConstraintInputs` took the full `Structured`, and the blockers it produces carry their
reasons into the stage-1 prompt. Safe by content (`hardconstraint.CVEvidence` is itself a
six-field whitelist) but not by type, so it takes the projection now too.

The RED test is worth keeping in mind for the rest of this list: a JSON blob with an extra
`date_of_birth` key reached the model *verbatim* through the blacklist. After the change that
test is not expressible at all, which is the fix — so the durable guard is a tripwire rather than
a filter: `professional_test.go` fails when a field is added to `Structured` and declared neither
in `Professional` nor in a `withheld` map recording why a model must not see it. A test that
cannot fail was deleted rather than kept for comfort (`TestCandidateProfileJSONCarriesNoContacts`,
whose property the return type now guarantees).

**S7** — `provider-taxonomy-keyless`, merged in #1403. `sources.Taxonomy()` names the transport-free registry and
is total over the adapter set; `All(client)` keeps the credential gate, so ingest still fails
config validation on a board file it cannot authenticate for. The finding got the cause right
and one consequence wrong in each direction. **Wrong, understated:** the live cost is not
theoretical — production holds 6,298 `whatjobs` postings, and none of them was ever eligible for
ATS-twin suppression on a keyless reindex host. **Wrong, overstated:** the missing
`SOURCE_VALUES` entries are cosmetic, not a broken facet. The source facet is `dynamic: true` and
reads the live distribution, and *nothing* in `web/src` imports `SOURCE_VALUES` or the `Source`
type — the generated constant had drifted unnoticed precisely because it is dead. The remedy also
grew one step: registering the three was not enough, because `All(nil)` still spelled two
different questions the same way, so the classifying call sites moved onto a named `Taxonomy()`.
Two adjacent label fixes came along — `/status` had its own local `titleCase`, a fifth surface
spelling facet codes its own way, which is the rule S1 settled.

**S6** — `resilient-derive-backfill`, merged in #1390. `cmd/backfill-derive` scans through
`worker.ResilientPage`. The exhaustion test had to move to the keyset cursor in the same
change: the degrade path returns a short page after skipping a damaged row, so the old
`< batchSize` check would have ended the scan there and reported a complete pass — worse
than the abort it replaces. A test pins it at 499-of-500 with the old check restored.

---

## Verdict

This codebase is in good architectural shape, and the honest headline is that most of the review's original hypotheses were wrong. The big shared seams that would normally rot — jobview as the one job wire type, internal/sources' capability-segregated HTTP client and its single date-parsing home for 186 adapters, internal/worker's bootstrap for 33 of 34 cron binaries, internal/llm as the only LLM construction path, cvedit as the only CV writer — all actually hold under grep. Nine reviewers looking for duplication mostly found deliberate, documented divergence, and the skeptic pass killed or downgraded roughly half of what they filed. What survives is not a structural rot story; it is a set of localized holdouts: a handful of call paths that bypass a shared thing that already exists, and a handful of load-bearing rules that are enforced by a comment or by constructor ordering where a type could enforce them. Two of those have already produced live defects (the incremental search pushers narrowing a role's cluster geography; two web label maps that disagree on SEO-indexed pages), and three more are latent fail-opens waiting for the next field to be added (the CV contact blacklist, the evidence gate's nil check, the manual-job derived columns). The one genuinely mis-cut boundary is small and specific: internal/handler serves a generated sqlc row as the public JSON of /api/v1/companies and never imports internal/pgconv, so the persistence vocabulary is held out of the wire per-endpoint rather than by a type. Nothing here calls for a re-cut of a package, a new framework, or a migration; the whole list is roughly a dozen small PRs.

---

## What holds

Findings are the visible half of a review. These are the seams that were attacked and did not
move — recorded so a future reader does not re-litigate them:

- jobview really is the one job wire type, verified rather than assumed: all 11 non-test importers use it, internal/search/document.go:34 embeds it so the index and the API cannot disagree, and even the wrapper responses nest it rather than flatten it (internal/handler/me_tracking.go:23 holds *jobview.Job; internal/handler/companies.go:49 uses []jobview.Job so the internal job id cannot leak). A grep for hand-rolled job JSON struct tags across the handler package finds nothing.
- internal/sources is the opposite of the '180 adapters each re-implementing fetch/parse' story: a repo-wide grep for time.Parse outside internal/sources/dates.go returns exactly one hit across 186 adapters, and transport is one client with capability-segregated interfaces (http.go:29-107) owning retry, 429/Retry-After, the 64 MiB body cap, AWS-WAF challenge detection and the SSRF guard, so adapters narrow to JSONGetter/HTMLGetter and stub one method in tests. Recently added adapters are 105-126 lines.
- The non-tech deletion rule is expressed so the two deleting paths cannot disagree: classify.ConfirmedNonTech(title, hasTechEvidence) (internal/classify/nontech.go:202) forces every caller to supply the tech veto, jobderive.TechEvidence is exported precisely for that, and both the ingest filter (internal/pipeline/catalogue_fit.go:52) and the prune rule (cmd/prune/rule.go:75-77) go through it. That is a rule enforced by an API signature rather than by a comment.
- internal/inbox.Queries (inbox.go:32-37) deliberately omits any mark-read method so Search cannot mark even by accident — a contract held by the type system rather than by convention. This is the standard several of the findings above ask the rest of the codebase to meet.
- internal/worker is a genuinely successful shared bootstrap: 33 of the 34 production cron binaries call worker.Bootstrap + worker.Main, per-main boilerplate is ~8 lines, and cmd/recount-companies/main.go is a complete working binary at 39 lines.
- cvedit really is the only CV writer: UpdateCV (internal/db/queries/cvs.sql:52) has exactly one caller, internal/cvedit/repository.go:89, inside the GetCVForEdit ... FOR UPDATE transaction, and internal/cv/Repository deliberately declares no document-writing method and says so.
- Error rendering is single-source: RenderError/classify (internal/handler/errors.go:48,66) is wired once in cmd/server, there are zero hand-rolled c.Status(...).JSON(fiber.Map{"error":...}) in the package, and the 20 per-feature xxxError mappers each translate only their own domain's sentinels and delegate the rest.
- The doc-comment culture is the reason this review could be evidence-based at all: most non-obvious decisions carry their reasoning in place (internal/atsboard/board.go:12-16 explains why the source key must be the provider key and what breaks otherwise; internal/flexjson names its two intentional siblings and the exact semantic each diverges on). Several reviewers dropped candidate findings after reading the comment that explained the decision — which is the system working.

---

## Themes

### 1. Transport has no company-shaped type

**Thesis.** internal/handler holds the persistence vocabulary out of the wire endpoint-by-endpoint rather than with a type: one list endpoint serves the generated sqlc row verbatim, the package never imports the shared pgconv converters and redeclares two of them, and one list endpoint re-implements the shared pagination clamp without its ceiling.

**Cost today.** `make sqlc` is currently an API-changing operation for GET /api/v1/companies — a column rename or SELECT alias change rewrites the public JSON with no compile error — and the Meili branch has to fabricate a fake db row (re-wrapping strings into pgtype.Text) to stay byte-compatible with it. The missing clamp is a live 500 on a public endpoint.

### 2. Invariants held by prose, wiring order, or an env var

**Thesis.** Five rules the code documents as load-bearing are enforced by a doc comment, by the order Register happens to run constructors in, or by whether a credential is set — where a constructor argument, a mapper, or a package move would enforce them. Three already have a call path that violates them.

**Cost today.** A moderator- or submission-authored vacancy lands with role_fingerprint and content_hash NULL, so it never clusters against the ATS copy of the same role and its semantic vector freezes after the first edit. The CV evidence gate is a post-construction mutator with two independent fail-open nil checks. The provider taxonomy silently loses three aggregators wherever their keys are unset, and the comment reasoning about that is wrong.

### 3. One decision, two implementations that have already drifted

**Thesis.** Genuine duplication where both copies encode the same decision, the copies must stay in step, and at least one has visibly diverged — as opposed to the many places in this repo where two similar-looking implementations differ for documented, real reasons.

**Cost today.** Two of these are user-visible today: a collapsed multi-city role loses the cities its reposts hold on every incremental push until the next full rebuild, and the same facet code renders under different names on the filter panel versus the SEO-indexed /insights pages.

### 4. The shared worker substrate has holdouts

**Thesis.** internal/worker is a successful shared bootstrap — 33 of 34 cron binaries use it — but the exceptions are precisely the workers doing the riskiest things, and two domain packages now import it just to reach one SQLSTATE constant that belongs in pgerr.

**Cost today.** The two LLM-spending backfills over stored user CVs are invisible to Sentry and drop their Langfuse trace flush on exactly the failed run; cmd/backfill-derive has no resume flag and no corruption tolerance, so one XX001 row makes a whole-catalogue re-derive permanently unfinishable; cmd/classify-mail exits 0 no matter how much mail dead-letters.

### 5. Docs that outran the code

**Thesis.** This repo's doc-comment and AGENTS.md culture is unusually good, which is exactly why the places where a doc now asserts something false are expensive: a reader trusts them and stops looking.

**Cost today.** internal/resumeextract/AGENTS.md sends anyone tracing 'what does the fit chain send' to experience.ProfessionalFrom, which no production code calls. docs/agents/mail-stack.md and cmd/classify-mail/store.go both assert the employer-reply reconcile 'has one home' while there are two implementations of its ordering rule.

---

## First three moves

The three changes that make the rest cheaper. Each is scoped to a single PR.

### Move 1

Delete insights.ts's CATEGORY_LABELS/SENIORITY_LABELS, import both from $lib/labels, move the differing overrides (fullstack → 'Full-Stack', the seniority set, the '' all-levels key) into labels.ts, and add a RELOCATION_LABELS there read by both enrichment.ts and facets.ts.

- **Why first:** It is the only finding on the list that is both S-effort and already user-visible: the same facet code renders under different names on the filter panel and on the SEO-indexed /insights pages. It is a pure deletion plus imports, needs no backend change, and it settles the rule labels.ts already claims ('Keeping ONE map prevents the drift…') before the next facet vocabulary forks the same way.
- **Unlocks:** Any future facet-label work has one place to land, and the remaining humanize question (sentence-case vs title-case) becomes a deliberate visual decision instead of an accident.

### Move 2

Put cmd/backfill-experience and cmd/backfill-resume-structured on worker.Main(run) + worker.Bootstrap(context.Background()), replacing their hand-rolled config.Load/context.Background/database.Connect and converting every os.Exit(1)/log.Fatalf to `return 1` so the deferred pool close and Langfuse flush actually run.

- **Why first:** These are the only two DB-writing production workers outside the shared path and both spend LLM money on stored user CVs, yet a failure in either is invisible to Sentry and backfill-experience drops its trace flush on exactly the run that failed. The change is a drop-in against an existing, well-used helper.
- **Unlocks:** Everything else about those two workers becomes diagnosable — including whether the résumé-extraction chain they duplicate is worth touching at all, a question currently unanswerable because failures leave no trace.

### Move 3

Add a per-(company_slug, role_fingerprint) cluster-geography query next to RoleClusterCount and call doc.MergeClusterGeography in the three incremental pushers (cmd/ingest/store.go, internal/linkimport, cmd/embed/indexer.go).

- **Why first:** It is the one live search defect in the list: because the Meili push is a field-level document update, an ingest content change on a collapsed multi-city canon overwrites the widened countries/regions/cities with the canon's narrow set, and the role stops being findable by its reposts' cities until the next full rebuild. Ingest already pays a per-row RoleClusterCount, so the 'a single row cannot ask' rationale does not apply.
- **Unlocks:** It forces the four identical FromJob/ClassifyReality preambles into one diff, which makes the follow-on question — whether a single search-package document constructor is worth it — concrete rather than speculative.

---

## Ranked shortlist

The top findings by *(cost of leaving it in) / (effort to fix)* — not by severity label alone.
Entries whose id contains a `+` are two area findings merged into one during synthesis; their
unmerged originals appear in the full catalogue below.

| # | Status | Finding | Area | Lens | Sev | Effort |
|---|---|---|---|---|---|---|
| 1 | ✅ | CATEGORY_LABELS and SENIORITY_LABELS are declared twice in the web app, against labels.ts's own 'keep ONE map' rule, and have already drifted | `web-frontend + cross-cutting` | reuse | medium | S |
| 2 | ✅ | Building a job's search document is re-implemented in four places, and only the full reindex applies the role cluster's geography union | `job-core` | reuse | medium | M |
| 3 | ✅ | The two LLM-spending backfill workers bypass worker.Bootstrap/worker.Main, losing Sentry, SIGTERM handling and the exit-code convention | `infra-workers + cross-cutting` | reuse | medium | S |
| 4 | ✅ | content_hash and role_fingerprint are comment-enforced columns each write path must remember; the moderator path sets neither and Telegram/link-import set only one | `job-core` | boundary | medium | M |
| 5 | ✅ | The evidence gate — the rule the tailoring capability exists to enforce — is an optional post-construction mutator with two independent fail-open nil checks | `cv-user` | coupling | medium | M |
| 6 | ✅ | worker.ResilientPage is the shared corruption-tolerant full scan over jobs, but only cmd/reindex uses it — two backfills run the identical keyset scan raw | `infra-workers` | reuse | medium | M |
| 7 | ✅ | sources.All gates adapter registration on env credentials, so the provider taxonomy silently loses three aggregators wherever those keys are unset | `ingest-sources` | coupling | medium | M |
| 8 | ✅ | The 'de-identified CV' seam is a JSON string, so the contact whitelist is enforced three ways — one of them the blacklist the codebase's own doc calls wrong | `ai-stack` | boundary | medium | M |
| 9 | ✅ | GET /api/v1/companies serves the sqlc-generated db.ListCompaniesRow as its public JSON contract, and the Meilisearch path hand-fakes that persistence type to match | `http-layer + cross-cutting` | boundary | medium | M |
| 10 | ✅ | JobDrawer re-implements two lib modules whose stated job is to be the single home for exactly that code | `web-frontend` | reuse | medium | S |
| 11 | ✅ | JobCopies hand-rolls limit/offset parsing and drops the int32 clamp the shared helper exists for | `cross-cutting` | reuse | low | S |
| 12 | ✅ | internal/handler never imports internal/pgconv and re-declares two of its converters locally | `cross-cutting` | reuse | low | S |
| 13 | ✅ | internal/handler carries two hand-rolled SSE stream writers with the same headers, deadline discipline and keepalive, already sharing one constant across the split | `ai-stack` | reuse | medium | M |
| 14 | ✅ | The db.Job → UpsertJobParams remap needed for jobhash.Of is copy-pasted verbatim into two backfill commands | `job-core` | reuse | low | S |
| 15 | ✅ | SQLSTATE classification is split between internal/pgerr and internal/worker, contradicting pgerr's own 'single home' claim — and two domain packages now import worker for it | `infra-workers` | boundary | low | S |
| 16 | ✅ | The application pipeline's forward order and its terminal set live in internal/mailclassify, not in internal/userjob which owns the stage vocabulary | `mail-notify` | boundary | medium | M |
| 17 | ✅ | atsdetect.FromURL is a second, drifted implementation of atsboard.Recognize despite atsboard's stated 'one definition' contract | `ingest-sources` | reuse | medium | L |
| 18 | ✅ | The employer-reply ledger reconcile — a two-statement ordering rule — is implemented once in internal/inbox and again in cmd/classify-mail | `mail-notify` | boundary | low | S |
| 19 | ✅ | experience.ProfessionalFrom is a second implementation of Store.Professional reachable from nothing but its own tests, and the docs still name it as the fit-analysis path | `cv-user` | simplicity | low | S |
| 20 | ✅ | cmd/classify-mail is the one worker that bypasses worker.ExitCode, because its Fail statement dropped the dead-letter signal its two siblings return | `ai-stack` | reuse | low | S |

### ✅ S1. CATEGORY_LABELS and SENIORITY_LABELS are declared twice in the web app, against labels.ts's own 'keep ONE map' rule, and have already drifted

`web-frontend + cross-cutting` · reuse · severity **medium** · effort **S** · id `38+42`

**Problem.** The same facet code renders under different names depending on which page the user is on, and the /insights pages are SEO surfaces whose titles and auto-intro sentences are built from the forked categoryLabel, so the mismatch is indexed. Seven multi-word category codes diverge on casing between the filter panel and the job-detail Category row; 'fullstack' is 'Full-Stack' on insights and 'Fullstack' everywhere else; relocation not_supported reads 'None' in the filter and 'Not supported' on the detail page. labels.ts names this exact class of drift as the reason it exists.

**Remedy.** Delete insights.ts's CATEGORY_LABELS and SENIORITY_LABELS, import both from $lib/labels, and move the entries whose label differs from the fallback (fullstack, the full seniority set, the '' all-levels key) into labels.ts. Add a RELOCATION_LABELS to labels.ts read by both enrichment.ts:191 and facets.ts:358, picking one wording. Leave enrichment.ts's sentence-case humanize alone — it is what its own doc comment specifies, and folding it into facets.ts's title-case is a visual change, not a drift fix.

**Evidence.**

- web/src/lib/labels.ts:1 — "Single source of display labels ... Keeping ONE map prevents the drift that previously left stale region codes and inconsistent casing in two places"
- web/src/lib/labels.ts:50 — CATEGORY_LABELS (10 entries), with ai_engineering: 'AI Engineer' at :52
- web/src/lib/insights.ts:15 — a second exported CATEGORY_LABELS (35 entries), with ai_engineering: 'AI Engineering' at :27
- web/src/lib/labels.ts:35 — SENIORITY_LABELS; web/src/lib/insights.ts:66 — a second SENIORITY_LABELS
- web/src/lib/facets.ts:305 — humanize() title-cases every word; web/src/lib/enrichment.ts:47 — a different humanize() capitalizes only the first letter; web/src/lib/insights.ts:79 — a third inline regex fallback
- web/src/lib/facets.ts:358 — RELOCATION not_supported: 'None' vs web/src/lib/enrichment.ts:31 — not_supported: 'Not supported'; every other facets.ts vocabulary imports its overrides from labels.ts

### ✅ S2. Building a job's search document is re-implemented in four places, and only the full reindex applies the role cluster's geography union

`job-core` · reuse · severity **medium** · effort **M** · id `14`

**Problem.** Four call sites independently answer 'how do I turn a db.Job into the document Meilisearch should hold', and they answer it differently. The reindex widens a canon's geography with its cluster's union; the three incremental pushers do not, and because the push is a field-level document update it replaces the widened countries/regions/cities with the canon's own narrow set. A collapsed multi-city role stops being findable by the cities its reposts hold until the next full rebuild. The identical 'repost, mass := 1, 1; if RoleClusterCount succeeds…' preamble in all four is the visible symptom of an assembly with no owner.

**Remedy.** Fix the narrowing first: add a per-(company_slug, role_fingerprint) cluster-geography query next to RoleClusterCount (only the whole-catalogue RoleClusterGeoAll exists today, internal/db/queries/jobs.sql:379) and call doc.MergeClusterGeography in the three incremental pushers. Ingest already pays a per-row RoleClusterCount, so the 'a single row cannot ask' rationale does not apply to it. Collapsing the four preambles into one search-package constructor is a reasonable follow-on, but pass it plain values (repost, mass, three geo slices) rather than injected lookup ports — cmd/embed deliberately skips reality in pgOnly mode.

**Evidence.**

- cmd/reindex/main.go:516-530 — repost/mass lookup → search.FromJob → jobview.ClassifyReality → doc.MergeClusterGeography
- cmd/ingest/store.go:153-171 — the same RoleClusterCount → degrade-to-(1,1) → FromJob → ClassifyReality block, with no MergeClusterGeography
- internal/linkimport/linkimport.go:281-296 — third copy, no geography merge
- cmd/embed/indexer.go:31-53 — fourth copy, no geography merge
- internal/search/client.go:496 — SubmitJobs uses UpdateDocumentsWithContext, a field-level update, and jobview's Cities/Countries/Regions carry no omitempty (internal/jobview/jobview.go:67), so the keys always overwrite
- internal/search/document.go:78-83 — MergeClusterGeography's doc says only the full reindex has the whole cluster in view

### ✅ S3. The two LLM-spending backfill workers bypass worker.Bootstrap/worker.Main, losing Sentry, SIGTERM handling and the exit-code convention

`infra-workers + cross-cutting` · reuse · severity **medium** · effort **S** · id `30+44`

**Problem.** These are the only two DB-writing, prod-data workers outside the shared path, and both spend LLM money on stored user CVs. Neither calls observability.Init, so a panic or run-ending error in either is invisible to Sentry — the exact gap worker.Main was written to close. backfill-experience's os.Exit(1) drops the buffered Langfuse traces that would explain why the run failed, and backfill-resume-structured exits 0 no matter how many users failed.

**Remedy.** Convert both mains to `worker.Main(run)` + `worker.Bootstrap(context.Background())` — a drop-in for the config.Load/Connect/defer-Close they hand-roll — and replace the os.Exit(1)/log.Fatalf sites with `return 1` so the deferred flush runs. Do not attempt to unify the two workers' résumé-extractor construction chains: they encode deliberately opposite policies (every piece fatal vs every piece optional), both documented in place.

**Evidence.**

- cmd/backfill-experience/main.go:62-68 — config.Load + context.Background + database.Connect hand-rolled
- cmd/backfill-experience/main.go:136 — os.Exit(1) fires with `defer pool.Close()` (:69) and `defer flush()` (:76) pending; os.Exit runs no deferred function, so the Langfuse trace flush is skipped precisely on the partially-failed run
- cmd/backfill-resume-structured/main.go:47-53 — the same hand-roll; :161 counts `failed` then returns normally, so the process exits 0 even when every user failed; log.Fatalf at :52/:64/:79/:108/:120/:126 each skips the same flush
- internal/worker/bootstrap.go:29-53 — Bootstrap does observability.Init, signal.NotifyContext(SIGINT/SIGTERM), pool, one cleanup
- internal/observability/AGENTS.md — "observability.Init lives in worker.Bootstrap"; only harvest-*/gen-contracts are declared out of scope
- verified: `grep -L worker.Bootstrap cmd/*/main.go` returns only these two plus cmd/server and nine DB-free dev tools

### ✅ S4. content_hash and role_fingerprint are comment-enforced columns each write path must remember; the moderator path sets neither and Telegram/link-import set only one

`job-core` · boundary · severity **medium** · effort **M** · id `12`

**Problem.** 'What columns a persisted job must carry' is split between the aggregate and two derived columns every caller is told, in a comment, to bolt on afterwards. Three of the four write paths forget at least one. A moderator- or submission-authored vacancy lands with role_fingerprint NULL, so it can never be deduped against the ATS copy of the same role — exactly the case a crowdsourced submission creates — and with content_hash NULL, after which a moderator edit leaves `NULL IS DISTINCT FROM NULL` = false and the semantic vector permanently describes the pre-edit text.

**Remedy.** Fix the ordering first: jobhash.Of hashes PostedAt (internal/jobhash/jobhash.go:39), and both tg-extract and linkimport override PostedAt AFTER UpsertParams() returns, so a ContentHash computed inside UpsertParams() would fingerprint the wrong posted_at. Fields already carries PostedAt, so have those two callers set Fields.PostedAt before mapping; then UpsertParams()/UpsertManualParams() can own both derived columns, and content_hash + role_fingerprint can be added to UpsertManualJob/UpdateManualJob's column lists.

**Evidence.**

- internal/job/job.go:168 — UpsertParams states the contract in prose: "columns a caller derives separately (ContentHash, RoleFingerprint, or a PostedAt supplied outside the aggregate) are set on the returned struct after this call"
- cmd/ingest/store.go:87,90 — the only caller that sets BOTH
- cmd/tg-extract/store.go:49 and internal/linkimport/linkimport.go:198 — RoleFingerprint only; ContentHash left zero → NULL
- internal/db/queries/jobs.sql:587 (UpsertManualJob) and :685 (UpdateManualJob) — the moderator/submission column lists contain neither
- internal/db/queries/jobs.sql:287-300 — RoleClusterCount filters `role_fingerprint <> ''`, so a NULL fingerprint never clusters
- internal/db/queries/semantic.sql:23 — the re-embed trigger is `semantic_embedded_hash IS DISTINCT FROM content_hash`; internal/handler/match_analysis.go:381-387 justifies the NULL stamp with "a non-board job with no content_hash is never re-crawled, so its text is stable" — which UpdateManualJob falsifies

### ✅ S5. The evidence gate — the rule the tailoring capability exists to enforce — is an optional post-construction mutator with two independent fail-open nil checks

`cv-user` · coupling · severity **medium** · effort **M** · id `22`

**Problem.** Reordering handler assembly, or building the CV handlers in any context that does not also build the assistant, silently disables the honest wall: an API-key holder editing as ActorAgent through PATCH /me/cvs/:id writes arbitrary claims into a CV with no citation, no compile error, and no test failure. The dependency is security-critical but the type system models it as optional, and the nil check is duplicated so even a correctly-wired-but-bank-less bankGate passes everything. Production is wired correctly today — this is a latent fail-open, not a live bug.

**Remedy.** Hoist the experience bank to one variable before internal/handler/handler.go:289, pass it into newCVHandlers, and construct the editor with the real gate at cv.go:92. Delete Editor.WithEvidenceGate and the `g.bank == nil` branch. Stop there — do not add a NoGate type or make NewEditor reject nil: requireEvidence already short-circuits for ActorCandidate (internal/cvedit/policy.go:161-163), so a nil gate is legitimate for the candidate-only test editors.

**Evidence.**

- internal/handler/cv.go:92 — every CV write goes through an editor built with an explicitly nil gate: `cvedit.NewEditor(cvedit.NewRepository(pool, queries), nil)`
- internal/cvedit/policy.go:157 — `if e.gate == nil { return nil }` — a nil gate silently means 'no evidence required'
- internal/handler/assistant_cv_tools.go:355-357 — `if g.bank == nil { return nil }` — a second independent fail-open on the same rule (g.bank is never nil in production)
- internal/handler/assistant.go:96 — the ONLY production wiring: `if cvH != nil && cvH.editor != nil { cvH.editor.WithEvidenceGate(bankGate{bank: h.experience}) }`
- internal/handler/cv_tailor.go:241 — PatchCV (mounted mw.key at internal/handler/cv.go:157) sets ActorAgent for any API-key caller; that path has nothing to do with the assistant
- internal/handler/handler.go:266 — an equivalent experience.Store is already constructed 23 lines before newCVHandlers at :289, so the stated reason for late binding (internal/handler/cv.go:90-91, internal/cvedit/editor.go:128-131: "the bank is wired after this") is false

### ✅ S6. worker.ResilientPage is the shared corruption-tolerant full scan over jobs, but only cmd/reindex uses it — two backfills run the identical keyset scan raw

`infra-workers` · reuse · severity **medium** · effort **M** · id `32`

**Problem.** The shared answer to an observed prod condition was written, tested, and then applied to exactly one of the three full-table scanners. cmd/backfill-derive is described in CLAUDE.md as the worker that re-derives every deterministic column in one keyset pass — a whole-catalogue pass is its entire purpose — and it has no resume flag, so a single XX001 row makes it permanently unable to finish past that id, re-failing at the same place every run.

**Remedy.** Wire cmd/backfill-derive only: widen its store interface to the three methods worker.jobQueries names, build worker.NewFullScanReader(q), and replace the loop at :244-256 with ResilientPage using the `lastID == afterID` exhaustion test. Do not add a third PageReader constructor for backfill-descriptions' source-scoped path — that is a one-off historical repair; its unscoped branch can use NewFullScanReader and its scoped branch can stay as-is.

**Evidence.**

- internal/worker/resilient.go:16-19 — ResilientPage exists because one damaged TOAST pointer fails an entire SELECT * page; :101-141 re-lists the window as bare ids and fetches rows one by one on SQLSTATE XX001
- internal/worker/resilient.go:52-64 — NewFullScanReader wraps exactly ListJobsByIDAfter / ListJobIDsAfter / GetJob
- cmd/reindex/main.go:153 and :338 — verified by grep to be the only non-test consumers
- cmd/backfill-derive/main.go:245-247 — raw ListJobsByIDAfter inside the producer goroutine; any error calls fail(e) at :251, cancelling the whole run
- cmd/backfill-descriptions/main.go:188-194 — the same raw scan, aborting at :119-121
- commit 1153d215 ("survive corrupted (XX001) rows in full-scan workers") records that a broken TOAST pointer already crashed a full facet reindex on this prod database

### ✅ S7. sources.All gates adapter registration on env credentials, so the provider taxonomy silently loses three aggregators wherever those keys are unset

`ingest-sources` · coupling · severity **medium** · effort **M** · id `7`

**Problem.** Two unrelated facts share one map: 'can this worker crawl the provider' (a runtime credential question) and 'what kind of source is this provider' (a static, compile-time fact). Every taxonomy consumer reads the second through the first. cmd/reindex's aggregator-duplicate suppression therefore drops whatjobs and reed from the aggregator set whenever the reindex host lacks those env vars — and unlike usajobs (a federal feed with no ATS twin, which the comment reasons about correctly), whatjobs is a CPC reseller whose entire inventory is resold copies of first-party ATS postings.

**Remedy.** Mirror the pattern already used six lines below in the same function for taleo/meta (registry.go:292-307): on the transport-free listing path (c == nil) register usajobs/reed/whatjobs with an empty credential, so All(nil) — the marker/taxonomy path used by ProviderKind, AggregatorProviders and FilterableProviders — is total, while the real-client crawl path keeps the credential gate. Roughly six lines. Do not move the credential to Fetch-time: that would let cmd/ingest start crawls that fail per board and cool them. Then fix cmd/reindex/main.go:420 and internal/pipeline/AGENTS.md:11 and regenerate contracts.ts. This needs a spec delta — openspec/specs/source-ingest/spec.md:998-1000 and the three adapter tests currently assert the opposite.

**Evidence.**

- internal/sources/registry.go:269 — `if key := os.Getenv("USAJOBS_API_KEY"); key != ""` guards registry["usajobs"]; :272 the same for REED_API_KEY; :282 for WHATJOBS_PUBLISHER_ID
- internal/sources/reed.go:72 and internal/sources/whatjobs.go:76 — both declare `aggregator()`
- cmd/reindex/main.go:420-425 — `aggregators := sources.AggregatorProviders(sources.All(nil))`, preceded by a comment asserting "usajobs is the one adapter sources.All only registers when USAJOBS_API_KEY is set" — factually wrong; reed and whatjobs are gated identically
- cmd/ghost-crosscheck/main.go:85-86 — a second, uncited consumer with the same leak
- internal/handler/status.go:123,134 — a keyless server renders those three providers as KindOther
- internal/pipeline/AGENTS.md:11 carries the stale one-adapter claim while internal/sources/AGENTS.md:12 says three — the doc drift is itself evidence the conflation confuses maintainers

### ✅ S8. The 'de-identified CV' seam is a JSON string, so the contact whitelist is enforced three ways — one of them the blacklist the codebase's own doc calls wrong

`ai-stack` · boundary · severity **medium** · effort **M** · id `17`

**Problem.** Three call sites answer 'what part of the candidate's CV may a model see?' and only one is typed. Because both Analyze and Input.StructuredResume take a bare string, nothing stops a caller passing the contact-bearing Structured, and ats_report.go:211 does exactly that. The safety net is a four-key blacklist — precisely the mechanism resumeextract argues against. The complement of Professional happens to be those four keys today, so nothing leaks; the defect fires the first time somebody adds a field to Structured (an address, a github, a date_of_birth), which ships to the gateway through the ATS review while matchanalysis correctly withholds it.

**Remedy.** Make handler.structuredResumeJSON return (resumeextract.Professional, bool) and have PostATSReport skip the LLM call when !ok, rather than teaching atscheck to test a zero value. Then Analyze takes resumeextract.Professional and marshals + truncates in reviewUserPrompt, stripContacts is deleted, Input.StructuredResume becomes resumeextract.Professional, and candidateContext collapses to marshal + TruncateRunes. No new package or interface — there are only three producers.

**Evidence.**

- internal/resumeextract/structured.go:57-64 — "The field set is a whitelist, deliberately... A blacklist — dropping the four known contact keys — would disclose that new field by default, which is the wrong way round"
- internal/atscheck/analyzer.go:106-125 — stripContacts is exactly that blacklist: `for _, k := range []string{"full_name","email","phone","links"} { delete(m, k) }`
- internal/handler/ats_report.go:211 — `blob, err := json.Marshal(st)` marshals the FULL resumeextract.Structured (contacts included); ats_report.go:83 hands that string straight to Analyze
- internal/matchanalysis/analyzer.go:61 — `StructuredResume string` is the field type; matchanalysis/AGENTS.md calls it "the resumeextract wire shape (contact fields stripped)" — enforced by convention, documented as a type
- internal/matchanalysis/analyzer.go:385-393 — candidateContext unmarshals the string back into Structured and calls .Professional() again: a full JSON round trip whose second projection is a no-op, since internal/handler/match_analysis.go:333 already marshalled a Professional
- internal/handler/me_profile.go:92 — the correct shape of the same seam, returning *resumeextract.Professional

### ✅ S9. GET /api/v1/companies serves the sqlc-generated db.ListCompaniesRow as its public JSON contract, and the Meilisearch path hand-fakes that persistence type to match

`http-layer + cross-cutting` · boundary · severity **medium** · effort **M** · id `1+41`

**Problem.** The whole reason internal/jobview exists is abandoned one endpoint over. A `make sqlc` regeneration after someone renames a column or changes a SELECT alias silently rewrites the public JSON of /api/v1/companies with no compile error anywhere, and the Meili branch — which sets fields one by one — would silently omit the new one, so the same request could return different bodies depending on which backend served it. The search-backed branch has to construct a fake persistence row purely to imitate a struct it has nothing to do with. Nothing is broken today; the exposure is the next sqlc regeneration.

**Remedy.** Local fix only: add a `companyListItem` struct in companies.go with plain Go types (*string for the nullables) plus `fromListRow(db.ListCompaniesRow)` and `fromDocument(search.CompanyDocument)`, serve []companyListItem from both branches, and delete companyRowFromDoc and pgText. Do NOT create an internal/companyview mirroring jobview — companies have one 6-field list row and one detail view, where jobview earns its package by being shared across list/detail/search/index. Retyping companyView's pgtype fields through internal/pgconv is a separate, cheap follow-up.

**Evidence.**

- internal/handler/companies.go:230 — `return listResponse(c, companies, total, limit, offset)` where companies is []db.ListCompaniesRow straight out of sqlc
- internal/db/companies.sql.go:307-314 — the generated struct whose json tags (slug, job_count, tagline, hq_country) ARE the public API
- internal/handler/companies.go:357-373 — companyRowFromDoc projects a search hit back onto the generated row "so the Meili path is byte-for-byte compatible with the Postgres path"
- internal/handler/companies.go:375 — pgText re-wraps plain strings into pgtype.Text purely so JSON null-ness matches the DB path
- internal/handler/companies.go:69-100 — companyView, the sibling detail projection, also exposes pgtype.Text/pgtype.Int4 as its wire fields
- grep of every listResponse call site (agent_search.go:48, jobs.go:75, search.go:98, swipe.go:50, recommendations.go:50, inbox.go:154) — every other list serves a projection; /companies is the sole exception
- internal/jobview/AGENTS.md:1 — "One type, projected from the job.Job aggregate, so the API surfaces cannot drift apart"

### ✅ S10. JobDrawer re-implements two lib modules whose stated job is to be the single home for exactly that code

`web-frontend` · reuse · severity **medium** · effort **S** · id `37`

**Problem.** The drawer duplicates the description CSS exactly, so the two blocks are one edit away from disagreeing — and the drawer's comment already points at a file that no longer owns the rule, which is how the next person will miss it. The scroll-lock bypass is a consistency defect rather than a live one (the z-50 drawer cannot have the z-40 header menu opened over it), but it is the same habit.

**Remedy.** Use `<JobDescription html={…} />` at JobDrawer.svelte:371 and delete the 417-463 style block; swap the $effect at 127-133 for lockScroll/unlockScroll. Both are drop-ins.

**Evidence.**

- web/src/lib/components/JobDescription.svelte:2-4 — "Reused by the job page (JobView), the tracker/drawer, and the tailor artifact panel — one home for the CSS so the description reads the same everywhere"
- verified: `diff <(sed -n '420,463p' JobDrawer.svelte) <(sed -n '17,60p' JobDescription.svelte)` is empty — 44 byte-identical lines
- web/src/lib/components/JobDrawer.svelte:371 — inlines `<div class="job-description text-sm leading-relaxed">{@html …}</div>` instead of `<JobDescription html={…} />`; only JobView.svelte:16 and lib/tailor/ArtifactPanel.svelte:17 import the component
- web/src/lib/components/JobDrawer.svelte:419 — the copy's comment says "Styles mirror JobView's .job-description", but JobView no longer holds those styles (grep finds no such rule); it renders <JobDescription> at :452
- web/src/lib/scrollLock.ts:12 — a reference-counted body lock, "the body only unlocks once every requester has released"
- web/src/lib/components/JobDrawer.svelte:127-133 — hand-rolls the lock, bypassing the refcount HeaderSearch.svelte:98 and HeaderMenu.svelte:106 use

### ✅ S11. JobCopies hand-rolls limit/offset parsing and drops the int32 clamp the shared helper exists for

`cross-cutting` · reuse · severity **low** · effort **S** · id `43`

**Problem.** GET /api/v1/jobs/:slug/copies?offset=3000000000 parses fine (Fiber's QueryInt is a plain strconv.Atoi, so 64-bit accepts it), then int32(3000000000) wraps to a negative and Postgres rejects the negative OFFSET — the endpoint 500s where every other list endpoint returns an empty page. The shared helper was written with a doc comment naming exactly this overflow, and this call site re-implements it without the clamp.

**Remedy.** Minimal: clamp the offset in copies.go the way the helper does, or add `pageParamsMax(c, defaultLimit, ceiling)` and call it from copies.go:39-40. Leave similar.go:32 alone — it parses no offset. The meta-envelope difference is unrelated and arguably correct (copies' total is a whole-cluster COUNT, a different quantity).

**Evidence.**

- internal/handler/handler.go:119-124 — "The offset is clamped into int32 range because the column binds as a Postgres int4, and an unbounded query value would otherwise overflow on the conversion"
- internal/handler/handler.go:131 — `offset = min(max(c.QueryInt("offset", 0), 0), math.MaxInt32)`
- internal/handler/copies.go:40 — `offset := max(c.QueryInt("offset", 0), 0)` — no MaxInt32 clamp
- internal/handler/copies.go:44 — `RowOffset: int32(offset)` feeds that unclamped value straight into the int4 param
- internal/db/jobs.sql.go:1464 — ListRoleClusterCopiesParams.RowOffset is int32

### ✅ S12. internal/handler never imports internal/pgconv and re-declares two of its converters locally

`cross-cutting` · reuse · severity **low** · effort **S** · id `46`

**Problem.** internal/pgconv is imported by 16 packages and states as its purpose that these conversions live in one place. internal/handler carries private twins under different names, so a reader grepping for the conversion finds three spellings and cannot tell whether they agree. Small in isolation, but it is the same habit that produced pgText in companies.go, and it is what keeps the pgtype vocabulary alive inside the transport layer.

**Remedy.** Delete tsFromPtr and int4Ptr, import internal/pgconv, and call pgconv.Timestamptz and pgconv.IntPtr at their two call sites. Two-function deletion, no new abstraction.

**Evidence.**

- internal/pgconv/pgconv.go:1-5 — "Repositories map their domain types across the persistence boundary through these helpers so the nil<->NULL and pgtype<->Go conversions live in exactly one place instead of being re-declared in every package that touches the database"
- internal/pgconv/pgconv.go:27 — `func Timestamptz(t *time.Time) pgtype.Timestamptz`; internal/handler/match_analysis.go:427-432 — body-identical `tsFromPtr`, used at :282
- internal/pgconv/pgconv.go:45 — `func IntPtr(n pgtype.Int4) *int`; internal/handler/hardconstraint_inputs.go:138-143 — body-identical `int4Ptr`, used at :78
- verified: `grep -n pgconv internal/handler/*.go` returns nothing — the largest package in the repo never imports it
- internal/jobview/jobview.go:363 — the sibling read-path package does use pgconv.Timestamptz/pgconv.TimePtr

### ✅ S13. internal/handler carries two hand-rolled SSE stream writers with the same headers, deadline discipline and keepalive, already sharing one constant across the split

`ai-stack` · reuse · severity **medium** · effort **M** · id `18`

**Problem.** Both long-lived SSE endpoints had to solve the same four problems (fasthttp's WriteTimeout racing the stream writer, a cleared deadline pinning the goroutine forever, bufio.Writer not being concurrency-safe against the keepalive ticker, the request-scoped Sentry hub dying before the writer runs). One solved it with a small type; the other copied the reasoning into an inline closure plus two package-level free functions. This is one subtle concurrency/deadline protocol, not two vendor-shaped implementations — the next fix to the deadline race has to be made twice, and nothing makes that obvious.

**Remedy.** Give sseStream.event a bool return (true on marshal failure — an unencodable frame is our bug, not a dead client), rewrite streamTurn over newSSEStream(w, conn, sseWriteTimeout), delete writeEvent/writeComment (assistant.go:635,645) and the inline mutex, and pass the keepalive interval as an argument. If the headers still itch, a four-line sseHeaders(c) next to sseStream is enough; leave the hub clone inline where its comment lives.

**Evidence.**

- internal/handler/match_analysis_stream.go:202-241 — the sseStream type owning mu/w/conn/timeout, with event/comment/write doing the framing, the per-write SetWriteDeadline and the Flush
- internal/handler/assistant.go:411-454 — the identical machinery hand-rolled inline: a write closure at :419 setting the same deadline, a bare `var mu sync.Mutex` at :433, and a keepalive goroutine at :437 that locks/re-arms/writes just like match_analysis_stream.go:123
- internal/handler/assistant.go:650 vs match_analysis_stream.go:219, and assistant.go:636 vs :225 — identical frame formats
- internal/handler/assistant.go:393-396 vs match_analysis_stream.go:77-80 — the same four c.Set header lines including the nginx comment; assistant.go:406-409 vs :90-93 — the same sentry hub clone with the same rationale
- internal/handler/match_analysis_stream.go:190 — sseWriteTimeout is declared here and used from assistant.go:421 and :448, so the assistant already reaches into the other file's constant while re-implementing the type that owns it
- drift: assistant.go:133 uses the named assistantKeepalive; match_analysis_stream.go:125 uses a bare 15*time.Second; handler/AGENTS.md:91 still contrasts writeEvent with `writeSSE`, a function that no longer exists

### ✅ S14. The db.Job → UpsertJobParams remap needed for jobhash.Of is copy-pasted verbatim into two backfill commands

`job-core` · reuse · severity **low** · effort **S** · id `16`

**Problem.** Four hand-maintained lists of the same column set exist, two of them byte-identical. jobhash.Of is documented as covering 'every value that ends up in the Meilisearch document' but already omits three fields the document carries. When a field is added to the hash, the two twins keep computing the old fingerprint, so their rewritten rows carry a hash the ingest path will not reproduce and the next crawl of each reports `changed` once spuriously.

**Remedy.** Move the row→hash-input mapping into internal/jobhash (which already imports db) as `OfRow(j db.Job, description string) string` and delete both copies. Do not route through job.FromRow → Fields → UpsertParams: FromRow decodes the enrichment JSONB and returns a per-row error, which is real work and a new failure path inside two throwaway backfill loops.

**Evidence.**

- cmd/backfill-descriptions/main.go:205 — `func hashParams(j db.Job, description string) db.UpsertJobParams` listing 19 fields
- cmd/backfill-justjoin/main.go:117 — the same function, verified byte-identical body and identical doc comment ("the exact indexed fields jobhash.Of fingerprints")
- internal/job/job.go:171 — Fields.UpsertParams already exists "so every write path (ingest, telegram extraction) shares one mapping instead of re-listing the columns"
- internal/jobhash/jobhash.go:23-49 — Of's own field list omits Cities, EnglishLevel and IsTech, which internal/jobview/jobview.go:67,77 shows the document does carry — the drift the duplication makes cheap

### ✅ S15. SQLSTATE classification is split between internal/pgerr and internal/worker, contradicting pgerr's own 'single home' claim — and two domain packages now import worker for it

`infra-workers` · boundary · severity **low** · effort **S** · id `35`

**Problem.** A piece of the persistence-error taxonomy sits in the bootstrap package. The predicted consequence has already happened: internal/enrich and internal/embed pull in the whole worker dependency tree for one three-line predicate.

**Remedy.** Move corruptDataSQLState and IsCorruptedRow into internal/pgerr as pgerr.IsDataCorrupted, next to IsUniqueViolation/IsForeignKeyViolation. internal/worker/resilient.go calls it at :109 and :125; the resilient-scan policy (which errors opt into the degrade path) stays in worker, only the SQLSTATE recognition moves.

**Evidence.**

- internal/pgerr/pgerr.go:1-5 — "It is the single home for the SQLSTATE constants the repositories and the central error handler share"
- internal/pgerr/pgerr.go:14-17,21-30 — the codes plus the errors.As(*pgconn.PgError) unwrap
- internal/worker/resilient.go:19 — `const corruptDataSQLState = "XX001"` in a second package; :24-27 repeats the identical errors.As body
- internal/worker/exit.go:1-5 — worker's own doc scopes it to "shared bootstrap and run-outcome plumbing"
- internal/enrich/runner.go:123 and internal/embed/runner.go:182 — both import internal/worker for nothing but IsCorruptedRow (grep confirms those are their only worker.* references), so two domain packages transitively depend on config, database and observability to classify one SQLSTATE

### ✅ S16. The application pipeline's forward order and its terminal set live in internal/mailclassify, not in internal/userjob which owns the stage vocabulary

`mail-notify` · boundary · severity **medium** · effort **M** · id `29`

**Problem.** 'How an application may move through its stages' is a tracking-domain rule decided by the mail-classification package, so the mail vocabulary and the application state machine are welded together. The one-directional test lets a real drift through: insert a stage into userjob.Stages (say take_home between screening and interview) and stageOrder gives it rank 0, which ranks below `applied` and is not in terminalStages, so any forward signal advances the application BACKWARD out of it — and silence.go and buckets.go go stale the same way.

**Remedy.** Move rank + terminal into internal/userjob (IsTerminal, Forward(current, target) bool) and leave signalStage — genuinely mail's own — in mailclassify, so AdvanceStage becomes signal→stage plus userjob.Forward. Rank cannot simply be the index in Stages (accepted/rejected/withdrawn sit after offer), so keep the explicit active-rank table inside userjob next to silenceThresholds, which already keys on the same five stages. Then extend classification_test.go into the missing direction: every stage in userjob.Stages is either ranked or terminal.

**Evidence.**

- internal/mailclassify/classification.go:86-88 — stageOrder, a second independent encoding of the pipeline rank (applied 1 … offer 5)
- internal/mailclassify/classification.go:94-96 — terminalStages, the settled-outcome set, defined in the mail package
- internal/mailclassify/classification.go:114-126 — AdvanceStage, the "strictly forward, never resurrect a settled application" rule
- internal/userjob/stages.go:5-11 — Stages is documented as "the ordered application-stage vocabulary (active stages then terminal) and the single source of truth", and reaches the SPA via cmd/gen-contracts/main.go:285 → contracts.ts:1014
- internal/userjob/silence.go:44-50 and internal/userjob/buckets.go:45-49 — two more tables hand-listing the same five active stages
- internal/inbox/mutate.go:226 and internal/maillink/decide.go:51 — both the interactive and the worker path ask mailclassify how an application may move
- internal/mailclassify/classification_test.go:12-23 — the only binding between the packages, and it checks one direction only

### ✅ S17. atsdetect.FromURL is a second, drifted implementation of atsboard.Recognize despite atsboard's stated 'one definition' contract

`ingest-sources` · reuse · severity **medium** · effort **L** · id `8`

**Problem.** The same question — which board does this URL address — is answered by two independent tables with different rules, and the tables have already diverged in three checkable ways. Any ATS added to atsboard is invisible to cmd/harvest-role and to atsdetect.Detect; anything atsdetect learns (icims/oracle/taleo/neogov/paycom) is invisible to contribution and link resolution. The claimed data harm is unproven — cmd/harvest-boards probes every candidate against the platform API before committing (main.go:69-78), so a portal slug would probe dead — so the cost today is a wasted probe plus a table nobody can reason about as one.

**Remedy.** Delete fromurl.go's 11 overlapping cases and have FromURL call atsboard.Recognize first, keeping only the five shapes atsboard deliberately excludes (icims, oracle, taleo, neogov, paycom) as atsdetect-local cases, with a comment in both packages saying atsdetect is the harvest-only extension of the shared table. Do NOT move those five into atsboard: it is the accept-set for internal/contribution, which pays for onboarded boards (board.go:12-16), so widening it is a money-affecting change that needs its own proposal. Before deleting the workday case, port workdaySite's job/details rejection — atsboard's modeHostPath lacks it.

**Evidence.**

- internal/atsboard/board.go:1-16 and AGENTS.md:9 — "One definition, three consumers… a host added once is recognised by all three"; board.go:62-126 is the 46-host table
- internal/atsdetect/fromurl.go:19 — FromURL reimplements the same URL→(provider, board) mapping for ~16 providers, 11 of which atsboard already covers (workday, smartrecruiters, greenhouse, lever, ashby, jazzhr, recruitee, pinpoint, careerplug, cornerstone, pageup)
- internal/atsdetect/fromurl.go:35-37 — `case host == "jobs.smartrecruiters.com": return "smartrecruiters", segs[0]` — exactly the bug internal/atsboard/board.go:30-32 documents and fixes with modePathPortal, applied at board.go:80-81
- internal/atsdetect/fromurl.go:11 — localeSegment `^[a-z]{2}-[A-Za-z]{2}$` vs internal/atsboard/board.go:288 — `^[a-z]{2}-[A-Z]{2}$`: same concept, two regexes, already divergent
- internal/atsdetect/atsdetect.go:26 — `reserved = map[string]bool{"embed": true}` vs internal/boardresolve/boardresolve.go:73 — the same rule re-encoded inline as `matched && b != "embed"`
- nothing in atsdetect.go, fromurl.go or their tests mentions atsboard at all, and no rationale for the split is recorded anywhere

### ✅ S18. The employer-reply ledger reconcile — a two-statement ordering rule — is implemented once in internal/inbox and again in cmd/classify-mail

`mail-notify` · boundary · severity **low** · effort **S** · id `28`

**Problem.** A rule the mail-stack doc treats as the ledger's load-bearing invariant (retract before insert, because data-modifying CTEs read the same pre-statement snapshot) exists as two copies, one of which is in a cmd/ main and therefore outside every domain package's test surface. The classification worker — the highest-volume writer of employer_reply events — is the copy furthest from the rule's documentation, and three separate doc sites assert the opposite.

**Remedy.** Extract `func ReconcileMailEvent(ctx, q EventRecorder, userID, emailID int64, mailSource string) error` into internal/inbox, with EventRecorder being just the two methods already listed at inbox.go:52-53. The two callers keep different error semantics — inbox.syncLedger stays best-effort/logged, the worker propagates inside its transaction — so return error and let each decide. If importing internal/inbox from cmd/classify-mail feels wrong, the equally cheap alternative is to fix the three comments to say there are two implementations of one ordering.

**Evidence.**

- internal/inbox/mutate.go:194-213 — syncLedger: appevent.SourceForMail, then RetractSupersededEmailEvent, then RecordEmailApplicationEvent, with "Order matters: the retraction must land before the insert" at :200
- cmd/classify-mail/store.go:140-153 — the identical three steps, same order, same comment, inside the worker's private dbStore.Save
- cmd/classify-mail/store.go:138-139 — asserts "The reconcile is the same statement the inbox's manual paths call; the rule has one home" — there are two homes
- docs/agents/mail-stack.md:107-119 — states the rule as "one reconcile with five callers"
- internal/appevent/appevent.go:2-3 — names internal/maillink as a recording path, but `grep -rn appevent internal/maillink/*.go` returns nothing
- internal/inbox/inbox.go:52-53 — the two reconcile methods are already isolated as a narrow interface pair that *db.Queries and a WithTx copy both satisfy

### ✅ S19. experience.ProfessionalFrom is a second implementation of Store.Professional reachable from nothing but its own tests, and the docs still name it as the fit-analysis path

`cv-user` · simplicity · severity **low** · effort **S** · id `24`

**Problem.** An exported function with a long rationale comment and seven dedicated tests that no production code path calls. It duplicates Store.Professional's rule, so a future change to the composition has two places to land and one of them is exercised only by tests that will keep passing. The AGENTS.md a reader consults first points at the dead one.

**Remedy.** Delete ProfessionalFrom and point its tests at Store.Professional (or at experienceFromBank, which is the part they actually exercise). Update internal/resumeextract/AGENTS.md:20 to name experience.Store.Professional.

**Evidence.**

- internal/experience/professional.go:58 — exported func with a 16-line doc comment defending its design
- verified by repo-wide grep: the only references are professional.go:42/58 and seven call sites in professional_test.go — no cmd/ or internal/ non-test caller
- internal/experience/professional.go:18 — Store.Professional does the same composition (out := st.Professional(); out.Experience = history) and is what every real caller uses
- internal/handler/match_analysis.go:321 — the fit chain calls bank.Professional(...)
- internal/resumeextract/AGENTS.md:20 — still claims "experience.ProfessionalFrom takes the work history from the bank and everything else from here, and that is the only candidate text matchanalysis sends"

### ✅ S20. cmd/classify-mail is the one worker that bypasses worker.ExitCode, because its Fail statement dropped the dead-letter signal its two siblings return

`ai-stack` · reuse · severity **low** · effort **S** · id `19`

**Problem.** The queue triplication itself is not a defect — sqlc cannot express one claim/fail statement over three tables — but the third copy silently lost information, and that loss propagates all the way to the process exit code. A mail queue that quietly stops working is exactly what worker.ExitCode exists to catch, and classify-mail is the lone bypass.

**Remedy.** Make FailEmailClassification `:one ... RETURNING attempts, failed_at` to match its two siblings, widen maillink.Store.Fail to (deadLettered bool, err error), have maillink.Runner.Run return a small Stats{Failed, DeadLettered} tallied where it already logs the bookkeeping error, and end cmd/classify-mail on worker.ExitCode(stats.Failed, stats.DeadLettered). Roughly ten lines. Do not extract a shared worker.Outcome for two callers of a dozen lines each — enrich tallies under a mutex because a wave runs concurrently, embed deliberately fails on context.Background().

**Evidence.**

- internal/db/queries/mail_classification.sql:103 — `FailEmailClassification :exec`, whose comment concedes it "Mirrors RecordEnrichmentFailure / RecordSemanticFailure" — but those are `:one ... RETURNING attempts, failed_at` (enrichment.sql:53, semantic.sql:113)
- internal/maillink/runner.go:69 — `Fail(ctx, outboxID, cause, maxAttempts) error`, with no dead-letter bool, unlike internal/enrich/runner.go:26 and internal/embed/runner.go:36
- internal/maillink/runner.go:131-138 — the Run loop keeps no tallies at all
- cmd/classify-mail/main.go:59-60 — logs "classify-mail: done" and returns 0 unconditionally, while worker.ExitCode is used by eight binaries (cmd/enrich:72, cmd/embed:85, cmd/notify:77, cmd/ingest:231, cmd/liveness:182, cmd/tg-ingest:56, cmd/remind:69, cmd/tg-extract:83)
- nothing else reads email_classification_outbox.failed_at, so a permanently dead-lettering mail queue is visible in journalctl only

---

## Full catalogue

All 46 surviving findings, grouped by area. Includes the ones the shortlist
merged or omitted, each with the skeptic's verification note. A verdict of `overstated` means
the defect is real but the original framing, severity or remedy was wrong — read the verifier
note before acting.

| Area | Findings | reuse | boundary | coupling | simplicity |
|---|---|---|---|---|---|
| HTTP transport layer | 6 | 2 | 2 | 2 | 0 |
| Source adapters and ingest | 5 | 4 | 0 | 1 | 0 |
| Job domain core and pipeline | 5 | 3 | 2 | 0 | 0 |
| LLM / AI subsystem | 4 | 2 | 1 | 0 | 1 |
| CV / user-profile domain | 4 | 2 | 0 | 1 | 1 |
| Mail, applications and notifications | 5 | 3 | 2 | 0 | 0 |
| Shared infrastructure and the 43 binaries | 6 | 3 | 2 | 1 | 0 |
| SvelteKit app and design system | 5 | 3 | 1 | 0 | 1 |
| Cross-cutting duplication sweep | 6 | 4 | 1 | 1 | 0 |

### HTTP transport layer

> **Reviewer's note on what is well-factored here.** Genuinely well-factored, and deliberately not flagged:\n\n- **jobview holds.** I checked all 11 non-test files that import it and grepped for hand-rolled job shapes (`json:\"title\"`/`json:\"public_slug\"` struct fields across the package). There is no second job projection anywhere — even `myJobResponse` (internal/handler/me_tracking.go:23) nests `*jobview.Job` rather than flattening fields into it, and `companyDetailResponse` (companies.go:55) does the same. `internal/search/document.go` embeds the same type, so the index and the API cannot disagree. The claim in internal/jobview/AGENTS.md is true.\n- **Error rendering is genuinely single-source.** `RenderError`/`classify` (internal/handler/errors.go:48, :66) is wired once in cmd/server, and I found zero hand-rolled `c.Status(...).JSON(fiber.Map{\"error\": ...})` in the package. The 20 per-feature `xxxError(err) error` mappers (mapCVError, trackingError, referralError, …) each translate *their own domain's* sentinels into `fiber.NewError` and delegate the rest — that is the correct shape, not duplication, and I did not flag it. The one wrinkle I decided was too small to report: `classify` hard-codes four `internal/inbox` sentinels centrally (errors.go:70-84) while `mailToolError` (assistant_inbox_tools.go:407) re-lists the same four for the model-facing rendering, so the inbox is the one feature whose vocabulary lives in the shared classifier.\n- **The per-feature `*Handlers` pattern holds** — 28 structs, each with its own `new<Feature>Handlers` and `register(api, mw)`. Only four routes bypass it (health, autofill-profile, autofill/run, tools/ws), which is the subject of one finding above rather than evidence the pattern failed.\n- **Not flagged: the ~50 `BodyParser` + 400 repetitions.** Three lines of Go idiom each; a generic helper would buy nothing and cost a type parameter.\n- **Not flagged: internal/handler having 90 non-test files.** Given cohesive per-feature structs, that is fine Go; the actual weight problem is the 2,400 lines of assistant tooling, which is a specific finding with a specific cause, not a general \"split the package\" complaint.\n- **Not flagged: the two `attachGhost` variants** (jobs.go:147 vs search.go:178). They read from deliberately different sources (rows in hand vs Postgres stamps the index cannot carry), both funnel into the shared `ghostEvidenceFor` and `jobview.ClassifyGhost`, and both carry doc comments explaining why. Documented, deliberate, correct.\n- **Not flagged: the credits check-then-debit rule** repeated at match_analysis.go:185, match_analysis_stream.go:50 and cv_tailor.go:71. It is three lines each and the three sites genuinely differ on when the debit lands (after cache / after the SSE writer / after the session mints). A `credits.Store.CanAfford` would tidy it, but the rule is not drifting.

#### 1. GET /api/v1/companies serves the sqlc-generated db.ListCompaniesRow as its public JSON contract, and the Meilisearch path hand-fakes that persistence type to match

`company-list-serves-sqlc-row-as-wire-shape` · boundary · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** The whole reason internal/jobview exists — "one type ... so the API surfaces cannot drift" — is abandoned one endpoint over. A `make sqlc` regeneration after someone renames a column or changes a SELECT alias in internal/db/queries/companies.sql silently rewrites the public JSON of /api/v1/companies with no compile error anywhere. The search-backed branch has to construct a fake persistence row (including re-wrapping strings into pgtype.Text via `pgText`) purely to imitate a struct it has nothing to do with, so a second data source is now shaped by the first source's SQL. And because the type is generated, it cannot be fed to cmd/gen-contracts, which is why web/src/lib/types.ts carries a hand-copied `CompanyListItem` that nothing keeps in sync.

**Remedy.** Do the local fix only: add a `companyListItem` struct in companies.go with plain Go types (`*string` for the nullables) plus `companyListItemFrom(db.ListCompaniesRow)` and `companyListItemFromDoc(search.CompanyDocument)`, delete `companyRowFromDoc` and `pgText`. Do NOT create internal/companyview for gen-contracts' sake — the detail endpoint already has a Go projection and is still hand-mirrored on the client, so contract generation is a separate question that this change neither causes nor solves.

**Evidence.**

- internal/handler/companies.go:230 — `return listResponse(c, companies, total, limit, offset)` where `companies` is `[]db.ListCompaniesRow` straight out of sqlc
- internal/db/companies.sql.go:307 — `type ListCompaniesRow struct { Slug string `json:"slug"`; ...; Tagline pgtype.Text `json:"tagline"`; HqCountry pgtype.Text `json:"hq_country"` }` — generated code whose json tags ARE the public API
- internal/handler/companies.go:357 — `func companyRowFromDoc(d search.CompanyDocument) db.ListCompaniesRow` projects a search hit back onto the generated row; doc at :333 says "so the Meili path is byte-for-byte compatible with the Postgres path"
- internal/handler/companies.go:375 — `func pgText(s string) pgtype.Text` exists solely to re-wrap plain strings into a pgx type so JSON null-ness matches the DB path
- internal/handler/companies.go:64 — the sibling detail endpoint DOES project: `companyView` "mirrors db.Company minus the purely-internal bookkeeping columns ... so those never leak"
- internal/handler/companies.go:49 — and its jobs go through jobview: "Its Jobs field is []jobview.Job, not []db.Job, so the internal job id cannot leak"
- web/src/lib/types.ts:156 — `CompanyListItem` is hand-maintained on the client because there is no Go projection type for cmd/gen-contracts to emit (contrast jobview.Job, which is generated into contracts.ts)

**Verifier.** Every citation checks out. internal/handler/companies.go:230 returns `listResponse(c, companies, ...)` where `companies` is `[]db.ListCompaniesRow` from sqlc; internal/db/companies.sql.go:307-314 is the generated struct whose json tags (`slug`, `job_count`, `tagline`, `hq_country`) are literally the public JSON of GET /api/v1/companies. companies.go:336-373 builds fake persistence rows from search hits (`companyRowFromDoc`) and companies.go:375 exists only to re-wrap plain strings into pgtype.Text. I grepped every `listResponse` call site (agent_search.go:48, jobs.go:75, search.go:98, swipe.go:50, recommendations.go:50, inbox.go:154) — every other list serves a projection (jobview.Job or a local struct); /companies is the sole exception, so this is not a house style, it is a one-off. It does contradict the doctrine internal/jobview/AGENTS.md states ("One type, projected from the job.Job aggregate, so the API surfaces cannot drift apart"). Two corrections to the framing, which is why I drop severity to medium: (a) the sibling `companyView` (companies.go:69-100) is NOT a clean projection either — it exposes pgtype.Text/pgtype.Int4/json.RawMessage as its wire fields, so the contrast is weaker than claimed; (b) the causal claim that web/src/lib/types.ts:156 is hand-copied "because there is no Go projection type for cmd/gen-contracts to emit" is wrong — `Company` at web/src/lib/types.ts:120 is equally hand-copied and it DOES have a Go projection (companyView); cmd/gen-contracts/main.go:88 emits from package paths and cannot see unexported handler types either way. Nothing here breaks today; the exposure is a future `make sqlc` after a column/alias rename.

#### ✅ 2. The extension's autofill endpoint runs raw SQL against users and cvs, bypassing sqlc and cv.Store — and picks a different CV than cv.Store.BaseCV deliberately picks

`autofill-raw-sql-bypasses-cv-store` · boundary · severity **low** · effort **S** · verdict **confirmed**

**Problem.** The autofill profile is assembled from a query that no other code shares and that disagrees with the domain's own definition of "the user's CV": it takes the most recently updated row including tailored copies, while cv.Store.BaseCV deliberately excludes tailored ones. A user who tailors a CV for a vacancy and then opens a form gets their tailored header autofilled into an unrelated application. Because the SQL is hand-written it is also invisible to sqlc — a column rename in migrations breaks it at runtime, not at build time — and it re-implements the document unmarshal that cv.Store already owns. The three orphan routes that hang off `*API` are the reason: with no feature handler, there was no natural place to hold `*cv.Store`, so the handler reached for the pool instead.

**Remedy.** Replace the two raw queries with `cv.Store` and `queries.GetUserByID` and let autofill hold those two dependencies (a small `autofillHandlers` + register is fine, and matches the package convention). Note the one behaviour change to decide explicitly: `BaseCV` returns ok=false for a user whose only CVs are tailored, where the current query still finds a header — either fall back to the most recent CV in that case or accept the empty header, but write the choice down. Leave `browserTools` on `API`; the assistant uses the same hub.

**Evidence.**

- internal/handler/autofill_profile.go:76 — `a.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID)` — the only raw SQL in the whole handler package (verified by grep for pool.Query/Exec across internal/handler)
- internal/handler/autofill_profile.go:80 — `SELECT data FROM cvs WHERE user_id = $1 ORDER BY updated_at DESC LIMIT 1`, then `json.Unmarshal` into `cv.Document` by hand
- internal/db/queries/cvs.sql:66 — `GetBaseCVByUser` selects `WHERE user_id = $1 AND NOT is_tailored ORDER BY updated_at DESC`, with a doc comment explaining that excluding on is_tailored (rather than `job_id IS NULL`) is "the whole point" because prune nulls the vacancy link
- internal/cv/store.go:255 — `func (s *Store) BaseCV(...) (Record, bool, error)` is the supported read; it also owns the document unmarshal (`unmarshalDocument`)
- internal/db/queries/users.sql:21 — `GetUserByID` already exists for the email read
- internal/handler/handler.go:84 and :88 — the `API` struct carries `browserTools` and `autofillPlanner`, feature dependencies on a struct AGENTS.md describes as holding "only the cross-cutting dependencies", because these three routes (handler.go:443, :449, :452) never got a feature handler

**Verifier.** The bypass is real and the citations are exact. `grep -n 'pool.QueryRow|pool.Query(|pool.Exec' internal/handler/*.go` (non-test) returns exactly two hits, both in autofill_profile.go — :76 `SELECT email FROM users WHERE id = $1` and :80 `SELECT data FROM cvs WHERE user_id = $1 ORDER BY updated_at DESC LIMIT 1`, followed by a hand `json.Unmarshal` into cv.Document, while internal/cv/store.go:255 `BaseCV` and internal/db/queries/users.sql:21 `GetUserByID` already exist. That is invisible to sqlc and duplicates the store's document decode, in a package whose AGENTS.md gives every feature a handler struct with its own dependencies. Two corrections that pull severity down from medium: (a) the headline harm is close to inert — internal/cv/store.go:320 creates a tailored CV as `CreateTailored(..., base.TemplateID, base.Document)`, a copy of the base document, so the tailored row's contact header is the base's header; the "tailored header autofilled into an unrelated application" scenario needs the header itself to have been edited on the tailored copy; (b) "delete the browserTools field from API" is wrong — `a.browserTools` is also passed to the assistant at handler.go:324, so it is genuinely shared, not an autofill-only dependency.

#### 3. The 13-value company facet vocabulary is enumerated five times inside ListCompanies and a sixth time in internal/search, so adding a facet is an eight-file edit

`company-facet-vocabulary-enumerated-five-times` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** One function spells out the same facet set five times, and internal/search already holds it once as data. Adding a facet means: a table row in search/company.go, a `facetValues` local, two positional argument lists (where a mis-ordered argument compiles fine and silently mis-routes the exact-count decision), two params structs, and two SQL queries. The two params structs are field-for-field identical apart from paging, which is the shape of a struct that wants to exist once. This is the same class of drift the file's own comment at :143 warns about ("Parse each facet once and feed both queries, so their WHERE clauses can't drift") — it solved it for the two SQL calls and left it unsolved everywhere else.

**Remedy.** Two small edits, no new abstraction: (1) compute `filtered := isCompanyFilter(...)` once at companies.go:169 and reuse the bool at :205; (2) add a `countParams(p db.ListCompaniesParams) db.CountCompaniesParams` mapper so the 13 field assignments are written once instead of twice. Leave the SQL and the search facet table as they are — the SQL predicates and the Meili attribute names are two different vocabularies that are allowed to be spelled separately.

**Evidence.**

- internal/search/company.go:120 — `var companyFacets = []struct{ param, attr string }{ {"collections","collections"}, ... {"subindustries","subindustry"} }` — the canonical param→attribute table, 13 entries, driving CompanyFilterFromValues at :141
- internal/handler/companies.go:149-161 — the same 13 param names spelled out again as thirteen `facetValues(vals, "...")` locals
- internal/handler/companies.go:169 — all 13 locals passed positionally to `isCompanyFilter(search, collections, regions, ...)` for the Meili-vs-Postgres routing decision
- internal/handler/companies.go:205 — the identical 13-argument `isCompanyFilter(...)` call again, this time for the exact-count-vs-estimate decision
- internal/handler/companies.go:178 — all 13 assigned into `db.ListCompaniesParams{...}`; :207 — all 13 assigned again into `db.CountCompaniesParams{...}`, which differs only by lacking Limit/Offset

**Verifier.** The enumeration count is right (companies.go:149-161 locals, :169 and :205 the two isCompanyFilter calls, :178 ListCompaniesParams, :207 CountCompaniesParams, plus search/company.go:120-133), but the finding's central danger argument is factually wrong. `isCompanyFilter` is declared at companies.go:236 as `func isCompanyFilter(search string, facets ...[]string) bool` and its whole body is `if search != "" {return true}` then `for _, f := range facets { if len(f) > 0 {return true} }` — it is order-independent, so a mis-ordered argument cannot "silently mis-route the exact-count decision". It cannot mis-route anything. The two calls being identical is a redundancy worth one local variable, not a positional hazard. "Eight-file edit" is also wrong: it is three files (internal/handler/companies.go, internal/db/queries/companies.sql, internal/search/company.go). The remedy is disproportionate on two counts: sqlc does not share a params struct across two queries (they are generated per query), and driving the Postgres param names off `search.CompanyFacetParams()` would make the SQL layer's arguments depend on the Meili attribute table — the wrong direction, and populating a generated struct by ranging over names needs reflection or a map.

#### ✅ 4. Shared services are constructed inside a feature handler's constructor and then extracted from it by Register — the opposite of what AGENTS.md states — forcing three post-construction setters and load-bearing ordering

`handlers-own-shared-services-forcing-two-phase-wiring` · coupling · severity **low** · effort **M** · verdict **overstated**

**Problem.** Nine shared services are owned by whichever handler happened to need them first, so Register has to dig them back out (`jobsH.moderation`, `cvH.cvStore`, `contributionsH.intake`, `authH.accounts`, `reportsH.report`, `telegramH.telegramBot`, `assistantH.store`, `inboxH.revokeGmailGrant`) — exactly the pattern the package's own AGENTS.md rules out. The consequences are concrete: three handler structs have fields that are nil for part of their lifetime, three `withX` setters exist that would not otherwise, and lines 242–363 of Register are a topological sort nobody can reorder safely (matchH before cvH before assistantH, then back into cvH). Every new cross-feature dependency adds another reach-in or another setter.

**Remedy.** Hoist the four that are actually misplaced: build `cv.Store`, `cv.Renderer`, `assistant.Store` and `moderation.Service` as Register locals and pass them into newCVHandlers / newAssistantHandlers / newJobsHandlers / newSubmissionHandlers. That alone deletes withAssistantSessions, removes the AGENTS.md contradiction, and kills the jobsH/cvH reach-ins. Fold cfg.LLM into newAssistantHandlers instead of withFollowUps. Leave withAccountDeletion, WithCodes and WithNotifier alone — they are conditioned on an SES client that does not exist at construction time.

**Evidence.**

- internal/handler/AGENTS.md — "Services shared across features (résumé store, profile, credits, CV store/renderer, match analyzer, contribution, moderation) are built once in `Register` and passed to each constructor; single-feature services are built inside the feature's constructor."
- internal/handler/handler.go:242 then :247 — `moderation.New(...)` is passed into `newJobsHandlers`, then reached back out as `jobsH.moderation` to build `submissionsH`
- internal/handler/handler.go:363 — `newReferralHandlers(referralSvc, cfg.Blob, cvH.cvRenderer, cvH.cvStore, photoStore)` — cv.Store and cv.Renderer are built inside newCVHandlers (cv.go:80, :103) and extracted here
- internal/handler/handler.go:290 — `contributionsH.intake`; :295 — `inboxH.revokeGmailGrant`; :343 — `authH.accounts.WithCodes(...)`; :350 — `reportsH.report.WithNotifier(...)`; :357 — `telegramH.telegramBot`
- internal/handler/handler.go:295, :325, :328 — three post-construction setters (`authH.withAccountDeletion`, `cvH.withAssistantSessions`, `assistantH.withFollowUps`) whose only purpose is to break construction cycles the ownership choice created
- internal/handler/cv.go:59 — `assistantSessions` is documented as "Assigned after construction"; internal/handler/auth.go:60 — accountdelete deps "are nil until withAccountDeletion wires them"

**Verifier.** There is a real, documented contradiction inside this, but the list of nine is padded. Confirmed: internal/handler/AGENTS.md names "CV store/renderer" among the services "built once in Register and passed to each constructor", yet cv.NewStore is at internal/handler/cv.go:80 and cv.NewTypstRenderer at cv.go:106, and Register digs both back out at handler.go:363. assistant.Store is likewise built inside newAssistantHandlers (assistant.go:74) and extracted at handler.go:325 — a genuine cycle the ownership choice created. moderation.New at handler.go:242 is already built in Register (inline) and only needs to be a local so submissionsH at :247 stops reaching into jobsH. Refuted parts: `inboxH.revokeGmailGrant` (handler.go:295) is a METHOD on inboxHandlers (gmail.go:260), not a service — AGENTS.md explicitly blesses it ("the Gmail grant revoked ... shared with GmailDisconnect via revokeGmailGrant"), so hoisting it is not the same move. `authH.accounts.WithCodes` (:343) and `reportsH.report.WithNotifier` (:350) are each a feature's OWN service being handed an optional SES dependency that only exists inside the `if cfg.AWSRegion != "" && cfg.NotifyEmailFrom != ""` block at :339-352 — hoisting the services does not remove that two-phase step; the conditional does. And `assistantH.withFollowUps(cfg.LLM)` at :328 breaks no cycle at all: cfg.LLM is available before construction, so it is simply a missing constructor argument. So "three setters exist only to break cycles the ownership choice created" is true of exactly one of the three.

#### 5. Three handler structs each hold a `searcher` and re-implement the same availability check, pagination guard and hit→jobview loop; the documented shared core is private to one of them

`search-list-preamble-split-across-three-handler-structs` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** Five endpoints (search, agent search, similar, swipe deck, recommendations) are all "a page of jobview.Job from Meilisearch", but the shared core only serves two of them because it is a method on one of the three structs that hold a searcher. SwipeDeck's own comment claims it "runs the same Meilisearch query as SearchJobs" while its hand-built params silently drop semantic_ratio — latent today only because the SPA sends the default value, and a live divergence the moment hybrid ranking is turned on. The ghost-signal attachment shows the same asymmetry: SearchJobs calls attachGhost, the other four search-backed lists show no badge at all.

**Remedy.** Extract one package-level helper — `func jobViews(hits []search.JobDocument) []jobview.Job` — and use it at search.go:94, similar.go:40, swipe.go:46, recommendations.go:46 and agent_search.go:41 (agent_search keeps its per-hit description formatting before the call). Optionally add `searchPage(c) (limit, offset int, err error)` for the shared window guard. Nothing else; leave the three handler structs holding their own searcher, and decide the ghost badge question on product grounds rather than folding it into a shared core.

**Evidence.**

- internal/handler/search.go:103 — `runJobSearch` is documented as "the single place the query is built, so the public and agent search endpoints cannot drift" — but it is a method on `*searchHandlers` and only /jobs/search and /agent/jobs/search reach it
- internal/handler/search.go:21, internal/handler/user_jobs.go:26, internal/handler/resume.go:35 — `searcher` is a field on three separate handler structs (searchHandlers, trackingHandlers, resumeHandlers)
- internal/handler/swipe.go:17 and :26 — SwipeDeck re-implements the nil-search 503 and the `offset+limit > maxSearchWindow` guard, then builds `search.SearchParams` inline at :32 with the same buildSearchFilter+searchSort pair
- internal/handler/recommendations.go:23 and :28 — Recommendations re-implements both guards a third time (degrading to an empty page rather than 503)
- internal/handler/search.go:94, similar.go:40, swipe.go:48, recommendations.go:48, agent_search.go:45 — the identical `views[i] = hit.Job` projection loop in five places
- internal/handler/swipe.go:32 — the inline SearchParams omits `SemanticRatio`, which runJobSearch reads from the request at search.go:116

**Verifier.** The three `searcher` fields are real (search.go:21, user_jobs.go:26, resume.go:35) and the `views[i] = hit.Job` loop does appear five times, but the five endpoints are not one query with five wrappers: SimilarJobs (similar.go:33) calls `SimilarJobs(ctx, id, limit)` with its own limit bounds and no pagination or filter at all, Recommendations (recommendations.go:42) calls `RecommendByVector(vec, filter, limit, offset)`, SwipeDeck (swipe.go:35) calls `Search` with an exclusion filter. What is genuinely shared is a two-line window guard and a three-line projection loop. The semantic_ratio "divergence" is weaker than claimed: swipe.go's own comment scopes the sameness explicitly — "the same Meilisearch query as SearchJobs (same facets, free-text q, and sort)" — it never claims the semantic ratio, and the deck has no reason to accept one. Citation slop: the SearchParams literal is at swipe.go:35, not :32 (:32 is `return err` inside the ExcludedJobIDs check). The remedy is the part I reject outright: a `jobFeed.page(c *fiber.Ctx, extra ...)` returning five values and folding three different backend calls behind a variadic is precisely the generic-abstraction-over-two-cases the project's rules forbid, and it would be harder to read than the five explicit handlers.

#### 6. The assistant's 2,400-line tool layer lives in internal/handler only because it reaches through sibling handler structs into their unexported service fields

`assistant-tools-reach-through-sibling-handlers` · coupling · severity **low** · effort **L** · verdict **overstated**

**Problem.** AGENTS.md justifies the sibling-handler dependency as "the tool and GET /me/profile share one assembly and cannot drift", and that argument is real for the five call sites that reuse a handler *method* (h.cv.tailoringContext, h.cv.cachedAnalysisCtx, h.profile.structuredCV, h.search.hydrateDescriptions, h.resume.coverageFor). It does not hold for the ~18 sites above, which do `h.<sibling>.<privateService>.<Method>` — pure service lookup through a struct that adds nothing. The cost is that 2,409 non-test lines of agent tooling (nine assistant_*_tools.go files) are pinned inside the HTTP package: they cannot move to their own package without exporting six handler structs' internals, so internal/handler carries the whole tool surface plus its 98-package fan-out. It also makes the assistant unconstructible without first constructing six other handlers, and adding a field to trackingHandlers is a change the assistant's tests feel.

**Remedy.** If touched at all: change newAssistantHandlers to take the plain services for the sites that only look one up (*jobtracking.Service, searcher, facetCounter, *cv.Store, *cvedit.Editor, *userprofile.Service, inbox.Service) and keep the sibling-handler pointer only for the five genuine method reuses (tailoringContext, cachedAnalysisCtx, structuredCV, hydrateDescriptions, coverageFor) — passing the struct, not a one-method interface per reuse, since there is no second implementation. Move `cvH.editor.WithEvidenceGate(...)` out of assistant.go:96 into Register where the ordering is visible. Do not create internal/assistanttools: nothing outside HTTP consumes those tools, so a 2,400-line package move is churn without a consumer.

**Evidence.**

- internal/handler/assistant.go:33 — `assistantHandlers` holds `search *searchHandlers`, `resume *resumeHandlers`, `tracking *trackingHandlers`, `cv *cvHandlers`, `profile *profileHandlers`, `mail *inboxHandlers`
- internal/handler/assistant_tracking_tools.go:26 — `h.tracking.tracking.SaveJob(...)`; same double hop at :32, :41, :69, :116, :119, :148, :157 — every one of them wants `*jobtracking.Service`, not the handler
- internal/handler/assistant_tools.go:112 — `h.search.facets.FacetCounts(...)`; :174 `h.search.search.Search(...)`; :152 and :297 nil-check the sibling's private field
- internal/handler/assistant_cv_tools.go:90 — `h.cv.cvStore.SetAutopilotReport(...)`; :216 `h.cv.cvStore.GetForModel(...)`; :318 `h.cv.editor.Commit(...)`
- internal/handler/assistant_profile_tool.go:47 — `h.profile.userProfile.Get(ctx, userID)`; internal/handler/assistant_inbox_tools.go:400 — `return h.mail.inbox, nil`
- internal/handler/assistant.go:96 — `newAssistantHandlers` mutates a struct it was merely handed: `cvH.editor.WithEvidenceGate(bankGate{bank: h.experience})`

**Verifier.** The mechanics are accurately reported: I counted the hops and they match — h.tracking.tracking at assistant_tracking_tools.go:26,32,41,69,116,119,148,157; h.search.facets/.search at assistant_tools.go:105,112,152,174,297; h.cv.cvStore/.editor/.jobReader/.matchAnalysisCache at assistant_cv_tools.go:90,216,318 and assistant.go:550,575,592 and assistant_interview_tools.go:124; h.profile.userProfile at assistant_profile_tool.go:47; h.mail.inbox at assistant_inbox_tools.go:397-400; and the constructor does mutate a handler it was handed at assistant.go:96. But the structure is a documented decision, not an accident: assistant.go:29-32 says outright "reaches the other feature handlers for the services its tools call — the assistant is a facade over the same use cases the HTTP surface exposes", and internal/handler/AGENTS.md repeats it for get_profile. Two of the claimed costs do not survive checking: (a) "adding a field to trackingHandlers is a change the assistant's tests feel" is false — the tests use named-field literals (assistant_tracking_tools_test.go:109 `&assistantHandlers{tracking: &trackingHandlers{tracking: jobtracking.New(repo)}}`), so a new field costs them nothing; (b) "unconstructible without six other handlers" describes Register only — every test builds a partial `&assistantHandlers{...}` already. Nothing about this blocks feature work or causes bugs, so "high" is not defensible; it is a wiring preference with a small real kernel.

### Source adapters and ingest

> **Reviewer's note on what is well-factored here.** The core hypothesis of this review — \"180 adapters that each re-implement fetch/parse/normalize\" — is mostly WRONG, and I want to say so plainly. The shared substrate is real and genuinely used:\n\n- Date parsing is fully centralised. `internal/sources/dates.go` funnels every parser through `NotFuture`; a repo-wide grep for `time.Parse(` outside dates.go returns exactly ONE hit (arbeitsagentur.go:170). That is a 185-of-186 hit rate on the hardest thing to keep consistent.\n- Transport is one client with capability-segregated interfaces (http.go:29-107). Retry, 429/Retry-After, the 64 MiB body cap with its non-transient BodyTooLargeError, the AWS-WAF challenge detection and the SSRF guard are written once; adapters narrow to `JSONGetter`/`HTMLGetter`/etc. so their tests stub one method. This is textbook accept-interfaces Go.\n- Detail fan-out (`fetchDetails`/`fetchDetailsStream`, helpers.go:20-66), DOM walking (html.go), schema.org JobPosting decoding (jsonld.go, schema.go), remote/work-mode mapping (helpers.go:71-122), sanitisation (sanitize.go) and plain-text→HTML reconstruction (plaintext.go) are all shared and widely called.\n- Adding a provider really is a small diff: the most recently added adapters are hibob.go (115 lines, of which ~40 are code), compleo.go (105), instaffo.go (126). One line in registry.go. That is the 30-line answer, not the 300-line one.\n- The optional-capability markers (`boardless`/`aggregator`/`selfClosing`/`fullCatalog`/`sweepGrace`, source.go:111-146) are a good seam: the sweep policy in cmd/ingest reads adapter-declared facts rather than a parallel hardcoded list, and each marker's doc states the soundness condition. `linksource.boardCoverage` reusing the ingest registry instead of writing 50 single-page adapters is the right call and is well argued in its own doc.\n\nThings I deliberately did NOT flag:\n- Per-provider structs that redeclare `addressLocality`/`addressCountry` (radancy, northstone, globalpayments, talentlyft, itechart…) rather than using schemaAddress. schema.go:38 explicitly warns that a board deliberately omitting the region must not use the shared `Location()`, and the variants differ in real ways (radancy blanks numeric regions, globalpayments has a Place.name fallback, several are 2-field). Five duplicated struct fields is cheaper than the wrong abstraction here.\n- The exported alias shims in sources (`SanitizeHTML`→`sanitizeHTML`, `IsRemote`→`isRemote`, `ParseDate`→`parseDate`, `ElementAttr`→`elementAttr`). Pure indirection, but it is the conventional Go way to keep the in-package name lowercase while giving siblings a stable surface, and it is 6 one-line functions.\n- The identical `greenhouseJobPath` regex in internal/sources/postingurl.go:22 and internal/linksource/greenhouse.go:30. Real duplication, but one regex, and the two answer different questions ((source, external_id) vs a resolved job); the practical consequence I could construct (EU greenhouse hosts absent from postingurl.go:44's host switch) is absorbed by catalogSlugForURL's second URL-match tier.\n- `internal/sources` also owning catalog identity (identity.go), the provider taxonomy (classify.go), posting-URL canonicalisation for the browser extension (postingurl.go) and the description sanitisation policy — this is why 23 packages import a 58k-LOC adapter package. It is a real seam smell (internal/handler and internal/moderation import the whole adapter tree for three small functions), but splitting it is churn with no behavioural payoff today and the AGENTS.md treats the placement as deliberate. Noting the seam rather than flagging it.\n- The `sources.All(nil)` transport-free marker path (5 nil-guard sites in registry.go, 4 consumers). It currently works — no constructor dereferences its client — and rebuilding 186 structs per /status request is not a cost that matters at this traffic. The one genuine defect it produces (env-gated adapters vanishing) is filed as the credentials/taxonomy finding instead.

#### 7. sources.All gates adapter registration on env credentials, so the provider taxonomy silently loses three aggregators wherever those keys are unset

`registry-conflates-credentials-with-taxonomy` · coupling · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** Two unrelated facts share one map: "can this worker crawl the provider" (a runtime credential question) and "what kind of source is this provider" (a static, compile-time fact). Every taxonomy consumer reads the second through the first. cmd/reindex's aggregator-duplicate suppression therefore drops whatjobs and reed from the aggregator set whenever the reindex host lacks those env vars — and unlike usajobs (which the code's own comment reasons about, correctly, as a federal feed with no ATS twin), whatjobs is a CPC reseller whose entire inventory is resold copies of first-party ATS postings. Those copies stay unsuppressed and surface as duplicates. The same leak has already produced a wrong artifact: the checked-in contracts.ts SOURCE_VALUES omits all three providers because whoever last ran codegen had no keys.

**Remedy.** Do not move the credential to Fetch-time — that makes cfg.Validate pass in a keyless env and lets cmd/ingest start crawls that fail per board, writing board_health failures and cooling boards (a behavioural regression the current gate exists to prevent, per registry.go:266-268). Instead mirror the pattern already used six lines below in the same function for taleo/meta/bayt/gulftalent (registry.go:292-307): on the transport-free listing path (c == nil) register usajobs/reed/whatjobs with an empty credential, so All(nil) — the marker/taxonomy path used by ProviderKind, AggregatorProviders and FilterableProviders — is total, while the real-client crawl path keeps the credential gate. ~6 lines. Then fix the wrong comment at cmd/reindex/main.go:420 and the stale internal/pipeline/AGENTS.md:11, and regenerate contracts.ts. Note this needs a spec delta: openspec/specs/source-ingest/spec.md:998-1000 and openspec/specs/whatjobs-source/spec.md:48-53 currently say the registry SHALL NOT contain the provider when the key is unset, and internal/sources/{usajobs,reed,whatjobs}_test.go assert exactly that against All(nil).

**Evidence.**

- internal/sources/registry.go:269 — `if key := os.Getenv("USAJOBS_API_KEY"); key != ""` guards `registry["usajobs"]`; :272 the same for REED_API_KEY, :282 for WHATJOBS_PUBLISHER_ID
- internal/sources/reed.go:72 — `func (reed) aggregator() {}`; internal/sources/whatjobs.go:76 — `func (whatjobs) aggregator() {}`
- cmd/reindex/main.go:425 — `aggregators := sources.AggregatorProviders(sources.All(nil))`, preceded at :420 by a comment asserting "usajobs is the one adapter sources.All only registers when USAJOBS_API_KEY is set" — reed and whatjobs are equally gated and equally aggregators
- cmd/gen-contracts/main.go:281 — `sources.FilterableProviders()` (which is `filterableProviders(All(nil))`, registry.go:65) feeds the checked-in SOURCE_VALUES
- web/src/lib/generated/contracts.ts:1012 — SOURCE_VALUES contains neither 'usajobs' nor 'reed' nor 'whatjobs', while sources/usajobs.yml, sources/reed.yml and sources/whatjobs.yml all exist and are crawled
- internal/handler/status.go:123 — `reg := sources.All(nil)` then `sources.ProviderKind(reg, r.Provider)` at :134; a keyless server renders those three providers as KindOther

**Verifier.** Every citation is real. internal/sources/registry.go:269/272/282 gate usajobs/reed/whatjobs on env vars; reed.go:72, whatjobs.go:76, usajobs.go:48 all declare aggregator(). cmd/reindex/main.go:420-425 does read the aggregator set through sources.All(nil), and its comment is factually wrong today — it asserts "usajobs is the one adapter sources.All only registers when USAJOBS_API_KEY is set" while reed and whatjobs are gated identically (git shows the comment was written when usajobs was the lone keyed adapter, #1158, and was never revisited). internal/pipeline/AGENTS.md:11 carries the same stale claim, while internal/sources/AGENTS.md:12 has been updated to three — the doc drift itself is evidence the conflation is confusing maintainers. There is a SECOND uncited consumer with the same leak: cmd/ghost-crosscheck/main.go:85-86 also does sources.AggregatorProviders(sources.All(nil)). internal/handler/status.go:123/134 confirmed verbatim. I downgrade high→medium on two counts the reviewer overstated. (a) The contracts.ts artifact is inert: web/src/lib/facets.ts:533 declares the source facet `dynamic: true` (distribution-driven, labelled via sourceLabel at :331), it never imports SOURCE_VALUES (only mentions it in a comment at :6), and the exported `Source` type has zero uses anywhere in web/src — so the drift at contracts.ts:1012 is a stale artifact, not a broken filter. (b) The duplicate-suppression harm requires the reindex host to lack WHATJOBS_PUBLISHER_ID, which is not verifiable from this repo. What is certain and live: the taxonomy is not total, the reasoning comment guarding it is wrong, and two separate binaries plus the public status page silently misclassify.

#### 8. atsdetect.FromURL is a second, drifted implementation of atsboard.Recognize despite atsboard's stated "one definition" contract

`two-url-to-board-recognizers` · reuse · severity **medium** · effort **L** · verdict **confirmed**

**Problem.** The same question — which board does this URL address — is answered by two independent tables with different rules, and the divergence already reaches committed data. A SmartRecruiters posting behind a portal segment (jobs.smartrecruiters.com/<portal>/<company>/<posting>) is board=<company> to atsboard and board=<portal> to atsdetect; harvest-role feeds the latter into a board file, so the harvest can onboard a board that is not the tenant. Any ATS added to atsboard is invisible to harvest-role and to atsdetect.Detect; anything atsdetect learns (icims/oracle/taleo/neogov/paycom) is invisible to contribution and link resolution. The greenhouse "embed" exclusion now lives in three places.

**Remedy.** Take only the first half of the proposed fix, and do not widen atsboard. Delete fromurl.go's 11 overlapping cases and have FromURL call atsboard.Recognize first, keeping only the five shapes atsboard deliberately excludes (icims, oracle, taleo, neogov, paycom) as atsdetect-local cases, with a comment in both packages saying atsdetect is the harvest-only extension of the shared table. Do NOT move those five into atsboard as new rows/modes: atsboard is the accept-set for internal/contribution, which pays for onboarded boards (board.go:12-16), so widening it is a money-affecting behaviour change that needs its own proposal. Before deleting the workday case, keep workdaySite's job/details rejection — atsboard's modeHostPath does not have it. The 'embed' rule is a two-line concern; leave boardresolve.go:73 alone (see finding 3).

**Evidence.**

- internal/atsboard/AGENTS.md:9 — "One definition, three consumers… It lives here so a host added once is recognised by all three"; internal/atsboard/board.go:62-126 is the 46-host table
- internal/atsdetect/fromurl.go:20 — `func FromURL(rawurl string) (provider, board string, ok bool)` reimplements the same mapping for ~16 providers, 11 of which atsboard already covers (workday, smartrecruiters, greenhouse, lever, ashby, jazzhr, recruitee, pinpoint, careerplug, cornerstone, pageup)
- internal/atsdetect/fromurl.go:35-37 — `case host == "jobs.smartrecruiters.com": … return "smartrecruiters", segs[0], true`, exactly the bug internal/atsboard/board.go:30-32 documents and fixes with modePathPortal ("Taking the first segment reads the portal slug as a board"), applied at board.go:80-81
- internal/atsdetect/fromurl.go:11 — `localeSegment = ^[a-z]{2}-[A-Za-z]{2}$` vs internal/atsboard/board.go:288 — `localeSegment = ^[a-z]{2}-[A-Z]{2}$`: the same concept, two regexes, already divergent
- internal/atsdetect/atsdetect.go:26 — `reserved = map[string]bool{"embed": true}` encodes "embed is not a greenhouse board"; internal/boardresolve/boardresolve.go:73 re-encodes the same rule inline as `matched && b != "embed"` because atsboard does not know it
- cmd/harvest-role/parse.go:62 — `provider, board, ok = atsdetect.FromURL(apply)`; the result is written into a seed that cmd/harvest-boards appends verbatim to sources/<provider>.yml

**Verifier.** Citations hold (fromurl.go:19 not :20, a one-line slip). The overlap is real and is not two vendors' JSON shapes: both tables answer the identical question (URL → provider+board keyed to the ingest external_id namespace) and 11 providers appear in both — greenhouse, lever, ashby, jazzhr, recruitee, pinpoint, careerplug, cornerstone(csod), pageup, smartrecruiters, workday. Crucially, nothing in atsdetect.go, fromurl.go or their tests mentions atsboard at all, while atsboard's package doc (board.go:1-16) and AGENTS.md:9-12 assert "One definition means a host added once is recognised by all three" — the code contradicts its own documented boundary, with no stated rationale for the split. The divergences are verified, in both directions: fromurl.go:35-37 takes segs[0] for jobs.smartrecruiters.com, which board.go:30-32 documents as a bug and fixes at board.go:80 + :174-183 via segmentBeforePosting/smartrecruitersPosting; localeSegment differs (fromurl.go:11 `[A-Za-z]{2}` vs board.go:288 `[A-Z]{2}`); and board.go:159-172 (modeHostPath) has no equivalent of workdaySite's job/details rejection. atsboard/AGENTS.md:42-43's claim that Taleo/Oracle "cannot be derived from a URL at all" is contradicted by fromurl.go:76-88, which derives both from the platform's own hosts. I downgrade high→medium because the claimed data harm is unproven: cmd/harvest-role's output is only a seed, and cmd/harvest-boards probes every candidate against the platform API before committing (main.go:69-78), so a SmartRecruiters portal slug would almost certainly probe dead and be dropped — the cost is a wasted probe, not a committed wrong board. The reviewer offers no evidence of a portal slug actually in sources/smartrecruiters.yml.

#### ✅ 9. boardresolve scans the same page twice with two recognizers and a verbatim-copied URL regex, and its trusted-set gate discards everything the first scan found

`boardresolve-double-scans-with-copied-regex` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** One HTML body is regex-scanned twice, and the two passes use different recognizers with different coverage. Because step 1's result is filtered through a four-provider `trusted` allowlist, everything atsdetect.FromURL uniquely knows how to parse (icims, oracle, taleo, neogov, paycom) is found and then thrown away — and step 2's atsboard.Recognize does not know those hosts at all. A company careers page whose only embedded ATS link is `careers-acme.icims.com` resolves to nothing, even though this repo contains a working parser for it (internal/atsdetect/fromurl.go:69-75). The `trusted` set is also a third place restating the "which provider keys match the external_id namespace" rule that atsboard/AGENTS.md:13 already declares as an invariant of the table itself.

**Remedy.** No restructure. If finding 2's delegation lands, boardresolve's step 2 becomes redundant with Detect's own fallback scan and can then be deleted along with its absURLRe copy and the `b != "embed"` guard — but that is a consequence of that fix, not a separate one. Independently, delete the unreachable "workable" entry at boardresolve.go:29 or add the atsdetect case that would justify it.

**Evidence.**

- internal/boardresolve/boardresolve.go:48 — `var absURLRe = regexp.MustCompile(`https?://[^\s"'<>)\\]+`)`, byte-for-byte identical to internal/atsdetect/atsdetect.go:31
- internal/atsdetect/atsdetect.go:44-49 — Detect already runs that scan itself: "falls back to scanning every absolute URL through FromURL — so any board shape FromURL understands is detected on a careers page without duplicating its host parsing here"
- internal/boardresolve/boardresolve.go:66 — `if provider, slug, ok := atsdetect.Detect(html); ok && trusted[provider] && slug != ""`, where trusted (boardresolve.go:25-30) is only greenhouse/lever/ashby/workable
- internal/boardresolve/boardresolve.go:72-76 — a second `absURLRe.FindAllString(html, -1)` loop over the same body, this time through `atsboard.Recognize`

**Verifier.** Every line citation is exact (boardresolve.go:25-30, :48, :66, :72-73; atsdetect.go:31, :44-49), and the two absURLRe declarations are byte-identical — so the page really is regex-scanned twice. But the two central claims do not survive reading the docs. (1) The `trusted` gate is not "a third restatement of atsboard's invariant": boardresolve.go:22-24 states it constrains atsdetect's output, which carries no namespace contract at all, whereas step 2's atsboard results are ungated precisely because atsboard's table does carry that contract (board.go:12-16). Two different guards on two different recognizers, not one rule copied. (2) "A page whose only ATS link is careers-acme.icims.com resolves to nothing" is true, but that is a deliberate, documented conservatism (the package doc at boardresolve.go:6-7: "Only providers whose (provider, board) matches how the ingest pipeline namespaces jobs.external_id are accepted"), and widening it is a missing feature, which this review explicitly excludes. The only hard duplication is one regexp line that cannot be shared because atsdetect's copy is unexported. One genuine detail the reviewer missed and which is worth more than the finding as written: "workable" at boardresolve.go:29 is dead — atsdetect can never return it (fromurl.go has no apply.workable.com case and atsdetect.go:16-21's matchers cover only greenhouse/lever/ashby).

#### 10. cmd/harvest-boards hand-writes the listing call for 33 platforms whose adapter already makes exactly that call, while the adapterProber seam that avoids it is used for 3

`harvest-probers-duplicate-adapter-listings` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** For every one of these platforms the endpoint URL, its response envelope, and the "is this board live" decision are maintained in two files that must agree. When a platform changes its API the adapter is fixed (it is the thing that breaks in prod) and the prober quietly keeps validating against the dead endpoint — harvest then silently finds zero live boards, which main.go:73 only catches when *every* probe fails. Several of these probers add nothing at all over `Fetch`: recruitee, pinpoint, breezy and personio each issue the identical single request the adapter issues, decode a subset of the same body, and return len(). The cheap-probe rationale (a limit=1 page, a separate company-name endpoint) genuinely applies to greenhouse, smartrecruiters and workday — not to the single-request platforms.

**Remedy.** At most, replace the two probers that genuinely buy nothing — recruiteeProber (prober.go:295-310) and pinpointProber (prober.go:313-332) — with adapterProber entries, and leave the rest: they are either materially cheaper than the adapter (breezy, personio, greenhouse, smartrecruiters, workday) or return a company name the adapter does not expose (workable, join, smartrecruiters). Do not export sources.greenhouseBaseURL to erase a 60-character const mirror in a run-once host tool; widening a package's API for that is the worse trade. If anything here deserves a fix on its own merits it is the swallowed probe errors (prober.go's `return "", 0, nil` on fetch failure) defeating main.go:73's all-probes-failed guard — a different, smaller finding.

**Evidence.**

- cmd/harvest-boards/adapter_prober.go:21-27 — `adapterProber.probe` calls `a.newSource().Fetch(ctx, sources.CompanyEntry{...})` and counts; its doc says "reusing the proven adapter is both correct and DRY"
- cmd/harvest-boards/prober.go:541-576 — the probers map: 33 bespoke prober types against 3 adapterProber entries (cornerstone, taleo, neogov)
- cmd/harvest-boards/prober.go:14 — `const greenhouseBoardsAPI = "https://boards-api.greenhouse.io/v1/boards"` with the comment "mirrors sources.greenhouseBaseURL, which is unexported; this tool lives outside the sources package"
- cmd/harvest-boards/prober.go:303 `https://%s.recruitee.com/api/offers/` vs internal/sources/recruitee.go:11 `recruiteeBaseURL = "https://%s.recruitee.com/api/offers/"`
- cmd/harvest-boards/prober.go:322 `https://%s.pinpointhq.com/postings.json` vs internal/sources/pinpoint.go:22, and prober.go:340 `https://%s.breezy.hr/json` vs internal/sources/breezy.go:48
- cmd/harvest-boards/prober.go:397 `https://%s.jobs.personio.com/xml` vs internal/sources/personio.go:51 `https://%s.jobs.personio.com`
- cmd/harvest-boards/prober.go:255 `https://apply.workable.com/api/v1/widget/accounts/%s?details=true` vs internal/sources/workable.go:9 `workableBaseURL`

**Verifier.** The line citations are all exact (prober.go:14, :255, :303, :322, :340, :397, :541-576; adapter_prober.go:21-27), but the load-bearing claim — that these probers "issue the identical single request the adapter issues" — is false for most of the named examples, and I checked each. breezy: prober.go:334-350 is one GET, but internal/sources/breezy.go:47-58 fans out a detail-page fetch per posting via fetchDetails — adapterProber would multiply probe cost by the posting count across every candidate slug. personio: prober.go:391-403 is one XML GET, while internal/sources/personio.go:49-75 fetches the default feed, then the English feed, then per-job detail pages as a fallback. workable: the prober hits the same URL, but it returns resp.Name (prober.go:250, :261) — the account name the adapter never surfaces, and adapterProber returns "" by construction (adapter_prober.go:20-27, doc: "These adapters expose no cheap employer name"). That leaves recruitee and pinpoint as genuine no-value duplicates (~15 lines each, both returning the slug as the name), out of 35 map entries. The claimed harm is also weaker than stated, in a way that argues against the remedy: probers swallow fetch errors as `return "", 0, nil`, so main.go:160-165 never increments `failures` and the "all probes failed" guard at :73 never fires — but adapterProber does exactly the same (adapter_prober.go:23-25), so routing through it fixes nothing. adapterProber also ignores the injected httpClient, so it cannot be stubbed in tests, unlike the bespoke probers.

#### ✅ 11. The "page until a page adds nothing new" walk is extracted for []string in html.go but hand-rolled five more times with the same three rules

`paged-enumeration-loop-reimplemented` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** Five adapters each re-derive the identical end-of-feed contract, and the copies have already picked up small independent variations (teamtailor's `newLinks`, neogov's `total` check, bayt's id-keyed dedup) that make it hard to tell an intentional difference from an omission. The rule that matters most — a later page failing must NOT abort the board, but page 1 failing must — is restated five times and is the kind of thing that quietly regresses; the fullCatalog variant of that rule (crawlAllPagedLinks, html.go:264) exists precisely because getting it wrong mass-closes a catalogue, and none of the five hand-rolled copies participate in that distinction.

**Remedy.** Leave the loops. peopleforce.go:58-76 is the one that is genuinely pagedLinks with a struct element, and even it is a ~18-line local loop, not a shared decision that will silently rot. If the concern is that the page-1-vs-later-page rule is stated five times, state it once as prose — a bullet in internal/sources/AGENTS.md next to the sweepGrace bullet (:14), pointing at html.go:247-266 as the canonical statement — and leave the code alone.

**Evidence.**

- internal/sources/html.go:270 — `func pagedLinks(ctx, get, maxPages, pageURL, links, failOnGap)`; the loop at :273-292 encodes the three rules: page-1 failure is a board error, a later-page failure keeps what was gathered, `added == 0` ends the walk
- internal/sources/peopleforce.go:58-76 — the same loop, line-for-line, collecting `peopleforceListing{URL,Title}` instead of strings, with its own `seen`, its own `added := 0`/`added++`/`if added == 0 { break }`
- internal/sources/bayt.go:92-116 — the same loop again, deduping by extracted job id rather than by URL
- internal/sources/teamtailor.go:48-77 — the same loop with a /jobs→root 404 fallback bolted into the fetch step and `newLinks` in place of `added`
- internal/sources/neogov.go:60-90 — the same loop over `GetTextWithHeaders` fragments, plus a `total` early-exit
- internal/sources/hhru.go:149-174 — the same loop over embedded page state, keyed by int64 vacancy id

**Verifier.** Every citation is accurate — I read all six loops (html.go:270-292, peopleforce.go:58-77, bayt.go:89-116, teamtailor.go:47-77, neogov.go:60-90, hhru.go:146-176) and they do repeat the same three rules. But the finding's own severity argument collapses on inspection. Only two adapters implement fullCatalog (geekjob.go:39, habrcareer.go:66) and both already use crawlAllPagedLinks (geekjob.go:56); none of the five hand-rolled loops is a fullCatalog provider, so "none participate in that distinction" describes correct code, not a latent mass-close. There is no live bug in any of the five. The variations the reviewer calls worrying are each documented in place and are not omissions: teamtailor.go:43-46 explains the /jobs→root 404 fallback AND mutates listPath so later pages follow it (state the proposed fetchPage closure would have to carry); neogov's `total` early-exit at :87 is an extra stop condition, not a missing one; hh starts at page 0 so `page == 0` is its first-page check; bayt keys on the extracted job id but appends a transformed absolute URL. Each also wraps its page-1 error with its own provider-prefixed message. Extracting `pagedDistinct[T]` with a `key func(T) string` plus a `failOnGap bool` flag over five fetch steps that differ in transport (GetHTML / GetTextWithHeaders / embedded-state decode), page origin, and stop conditions is exactly the premature generic the project's stated principles rule out — it would be harder than the 15-line loop it replaces.

### Job domain core and pipeline

> **Reviewer's note on what is well-factored here.** Genuinely well-factored in this area, and deliberately not flagged:

- **jobderive is a real single owner.** `jobderive.Derive` has exactly one non-test caller in the live write path (`job.New`, internal/job/job.go:127), and all four write paths — ingest (pipeline.go:576), tg-extract (store.go:29), linkimport (linkimport.go:178), moderation (moderation.go:244) — reach it through the guarded `job.New` door with the aggregate's unexported state. That is the load-bearing invariant the docs claim, and it holds. My backfill finding is the one place it does not.

- **The non-tech catalogue rule is properly shared, and the ordering is enforced by the type of the API, not by a comment.** `classify.ConfirmedNonTech(title, hasTechEvidence)` (classify/nontech.go:202) forces every deleting caller to supply the tech veto; `jobderive.TechEvidence` (jobderive.go:205) is exported precisely for that; the ingest filter (pipeline/catalogue_fit.go:52) and the prune rule (cmd/prune/rule.go:75-77) both go through it. This is textbook — a rule that two deletion paths must agree on, expressed so they cannot disagree.

- **jobreality / ghost / liveness / jobmatch / facetsnapshot are clean pure classifiers** over scalars with no DB and no clock of their own, and each has exactly one reason to exist. `jobview.ClassifyReality` is correctly the single adapter from db.Job into jobreality.Input. I found no duplication of the classification logic itself — only of the plumbing around it (finding 3).

- **`pipeline.Runner` is a real seam, not a 12-step procedure.** Registry + Store + optional BoardHealth, bounded concurrency, per-board failure isolation, and the streaming/buffered split sharing one `saveOne`. The catalogue filter, the cooldown gate and the recovery probe are each one small named function. I have a complaint about how the optional store capabilities are typed (finding 4), not about the Runner's shape.

- **Dedup IS applied at ingest, reindex and notify now.** `jobdedup.CanonicalForRole` runs inside the ingest write transaction (cmd/ingest/store.go:106) and inside linkimport (linkimport.go:215) with the deliberate refuse-a-newer-canon rule that keeps it in step with `RecomputeRoleDuplicatesForCompany`; the reads (companies.sql, insights.sql, enrichment.sql, semantic.sql, jobs.sql) all filter `duplicate_of IS NULL`. The historical "dedup in only one of the three" bug is closed. The residual gap is that manual/submission rows carry no fingerprint at all, which is finding 1.

- **Not flagged: `vocab`.** It is a dependency-free vocabulary package with a test asserting Tech/NonTech/other partition CategoryValues exactly. Nothing to re-cut.

- **Not flagged: the pipeline's `Stats`/`RunStats` shape, the board_health backoff, or `cmd/prune`'s three-rule structure.** All small, single-purpose, and documented with the reasoning behind each threshold. cmd/prune in particular splits scan/plan/rule/retire/boards cleanly and the rule file is a pure function.

- **Deliberately not flagged as a finding: `jobhash.Of` omitting cities/english_level/is_tech** despite claiming to cover the search document. It is a known defect rather than a structural one, and it is cited as evidence inside finding 5 where the duplication that makes it hard to fix actually lives.

#### 12. content_hash and role_fingerprint are comment-enforced columns each write path must remember; the moderator path sets neither and Telegram/link-import set only one

`post-hoc-hash-columns-have-no-owner` · boundary · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** "What columns a persisted job must carry" is split between the aggregate (job.Fields.UpsertParams / UpsertManualParams) and two derived columns every caller is told, in a comment, to bolt on afterwards. Three of the four write paths forget at least one. A moderator- or submission-authored vacancy lands with role_fingerprint NULL, so it is invisible to RoleClusterCount, to RecomputeRoleDuplicatesForCompany and to the reality signal — it can never be deduped against the ATS copy of the same role, which is exactly the case a crowdsourced submission creates. It also lands with content_hash NULL: the first embed stamps semantic_embedded_hash = content_hash = NULL, after which a moderator edit of the description leaves `NULL IS DISTINCT FROM NULL` = false and the semantic vector never refreshes, permanently describing the pre-edit text.

**Remedy.** The remedy as written is subtly broken: jobhash.Of hashes PostedAt (internal/jobhash/jobhash.go:39 `write(timestamp(p.PostedAt))`), and both tg-extract (cmd/tg-extract/store.go:47-48) and linkimport (internal/linkimport/linkimport.go:195-197) override PostedAt AFTER UpsertParams() returns — so a ContentHash computed inside UpsertParams() would fingerprint the wrong posted_at and never match what a later ingest re-crawl produces. Fix the ordering first: Fields already carries PostedAt (moderation does `f.PostedAt = in.PostedAt`), so have those two callers set Fields.PostedAt before mapping; then UpsertParams()/UpsertManualParams() can own both derived columns, and content_hash + role_fingerprint can be added to UpsertManualJob/UpdateManualJob's column lists.

**Evidence.**

- internal/job/job.go:168 — UpsertParams docs the contract in prose: "columns a caller derives separately (ContentHash, RoleFingerprint, or a PostedAt supplied outside the aggregate) are set on the returned struct after this call"
- cmd/ingest/store.go:87 and :90 — the only caller that sets BOTH ContentHash (jobhash.Of) and RoleFingerprint
- cmd/tg-extract/store.go:49 — sets RoleFingerprint only; ContentHash is left zero, so the row stores NULL
- internal/linkimport/linkimport.go:198 — same: RoleFingerprint only, no ContentHash
- internal/db/queries/jobs.sql:587 (UpsertManualJob) and :685 (UpdateManualJob) — the moderator/submission write path's INSERT and UPDATE column lists contain neither content_hash nor role_fingerprint
- internal/db/queries/semantic.sql:23 — the re-embed trigger is `semantic_embedded_hash IS DISTINCT FROM content_hash`, and semantic.sql:84 stamps `semantic_embedded_hash = content_hash`
- internal/db/queries/jobs.sql:295 — RoleClusterCount requires `role_fingerprint <> ''`, so a NULL fingerprint never clusters

**Verifier.** Every citation is exact. internal/job/job.go:168 states the prose contract; cmd/ingest/store.go:87,90 is the only site setting both; cmd/tg-extract/store.go:49 and internal/linkimport/linkimport.go:198 set RoleFingerprint only; UpsertManualJob (jobs.sql:587) and UpdateManualJob (jobs.sql:685) list neither column — I read both column lists in full. Both consequences check out: RoleClusterCount (jobs.sql:287-300) filters `role_fingerprint <> ''`, and manual jobs are minted by moderation.Create via UpsertManualJob (internal/moderation/repository.go:48) including the submission-approval path (internal/handler/submissions.go:16), so a curated copy of an ATS-crawled role never clusters or dedups. The content_hash arm is stronger than the reviewer knew: internal/handler/match_analysis.go:381-387 justifies the NULL-stamp rule with "a non-board job with no content_hash is never re-crawled, so its text is stable" — an assumption UpdateManualJob (an editable manual job) directly falsifies, and semantic.sql:23/84 then freezes the vector. Not high: manual/submission jobs are a small slice of the catalogue and the missing fingerprint degrades the reality signal to the neutral (1,1) rather than to a wrong value.

#### 13. Building a job's search document is re-implemented in four places and only the full reindex applies the role cluster's geography union

`search-doc-assembled-four-times` · reuse · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** Four call sites independently answer "how do I turn a db.Job into the document Meilisearch should hold", and they answer it differently. The reindex widens a canon's geography with its cluster's union; the three incremental pushers do not, and because the push is a field-level document update, an ingest content change on a collapsed multi-city canon replaces the widened countries/regions/cities with the canon's own narrow set. The role stops being findable by the cities its reposts hold until the next full rebuild. The identical "repost, mass := 1, 1; if RoleClusterCount succeeds…" preamble in all four is the visible symptom: the assembly is a shared decision with no owner.

**Remedy.** Fix the geography narrowing first, and keep it small: add a per-(company_slug, role_fingerprint) cluster-geo query next to RoleClusterCount (only the whole-catalogue RoleClusterGeoAll exists today, jobs.sql:379) and call doc.MergeClusterGeography in the three incremental pushers. Collapsing the four preambles into one search-package constructor is a reasonable follow-on, but pass it plain values (repost, mass, and the three geo slices, any of them zero) rather than inventing injected lookup ports — cmd/embed deliberately skips reality entirely in pgOnly mode (cmd/embed/indexer.go:40-53), so a lookup-injection seam would exist to serve one caller's nil.

**Evidence.**

- cmd/reindex/main.go:520 — `search.FromJob` → ClassifyReality (:524) → `doc.MergeClusterGeography(...)` (:530)
- cmd/ingest/store.go:155 — the same RoleClusterCount → degrade-to-(1,1) → FromJob (:163) → ClassifyReality (:167) block, with no MergeClusterGeography
- internal/linkimport/linkimport.go:283 — third copy of the same block; FromJob at :289, ClassifyReality at :294, no geography merge
- cmd/embed/indexer.go:43 — fourth copy; FromJob at :33, ClassifyReality at :51, no geography merge
- internal/search/document.go:78 — MergeClusterGeography's doc says it is "Called by the full reindex (which alone has the whole cluster in view); the single-row FromJob cannot"
- internal/search/client.go:496 — SubmitJobs uses UpdateDocumentsWithContext, so the pushed countries/regions/cities keys overwrite whatever the previous full reindex widened them to

**Verifier.** All four sites verified line-exact and read in full: cmd/reindex/main.go:515-531 (count → FromJob:520 → ClassifyReality:524 → MergeClusterGeography:530), cmd/ingest/store.go:153-171, internal/linkimport/linkimport.go:281-296, cmd/embed/indexer.go:31-53 — identical `repost, mass := 1, 1` degrade-then-FromJob-then-ClassifyReality preamble, only the reindex merges cluster geography. The harm mechanism is real: internal/search/client.go:496 uses UpdateDocumentsWithContext (field-level update) and jobview's Cities/Countries/Regions carry no omitempty (internal/jobview/jobview.go:67), so the keys are always present and always overwrite. I checked the counter-argument that this is a documented boundary — document.go:82-83 says the merge is "Called by the full reindex (which alone has the whole cluster in view); the single-row FromJob cannot" — but that documents why FromJob itself does not merge, not that an incremental push may narrow an already-widened doc; and ingest already pays a per-row RoleClusterCount, so "a single row cannot ask" is not true. Staying at medium rather than high: cmd/reindex always rebuilds and swaps a full index (main.go:186-196), so every narrowing self-heals on the next scheduled rebuild.

#### 14. The db.Job → UpsertJobParams remap needed for jobhash.Of is copy-pasted verbatim into two backfill commands, bypassing job.FromRow + Fields.UpsertParams

`hashparams-twins-bypass-fields-mapping` · reuse · severity **low** · effort **S** · verdict **confirmed**

**Problem.** Four hand-maintained lists of the same column set exist, two of them byte-identical. jobhash.Of is documented as covering "every value that ends up in the Meilisearch document (see internal/search.FromJob)" but already omits cities, english_level and is_tech, which the document does carry — the drift the duplication makes cheap. When a field is added to the hash, the two hashParams twins keep computing the old fingerprint, so their rewritten rows carry a hash the ingest path will never reproduce and every re-crawl of them reports `changed`, re-pushing the board to Meilisearch on each pass until the crawl overwrites the value.

**Remedy.** Move the row→hash-input mapping into internal/jobhash (which already imports db) as e.g. `OfRow(j db.Job, description string) string`, and delete both copies. Do NOT route through job.FromRow → Fields → UpsertParams as proposed: FromRow decodes the enrichment JSONB and returns an error per row (internal/job/repository.go:44-48), which is real work and a new failure path inside two throwaway backfill loops, to reach a mapping the hash package can own directly. Also drop the moderation/repository.go:86 leg from this finding — that method fills UpdateManualJobParams (a different column set: no URL/Source/ExternalID/PublicSlug, but with Cities/IsTech/EnglishLevel), so it is not "the same 19 columns"; the fair point there is only that Update hand-lists while its sibling Create uses f.UpsertManualParams.

**Evidence.**

- cmd/backfill-descriptions/main.go:205 — `func hashParams(j db.Job, description string) db.UpsertJobParams` listing 19 fields
- cmd/backfill-justjoin/main.go:117 — the same function, identical body and identical doc comment ("the exact indexed fields jobhash.Of fingerprints")
- internal/job/repository.go:34 — job.FromRow already maps db.Job → the domain aggregate
- internal/job/job.go:171 — Fields.UpsertParams already maps it back, and its own doc claims it exists "so every write path (ingest, telegram extraction) shares one mapping instead of re-listing the columns"
- internal/moderation/repository.go:86 — a third bypass: Update re-lists the same 19 columns inline into UpdateManualJobParams rather than through a Fields method
- internal/jobhash/jobhash.go:23 — Of's own field list is the fourth hand-maintained copy of "which columns are indexed content"

**Verifier.** cmd/backfill-descriptions/main.go:205 and cmd/backfill-justjoin/main.go:117 are byte-identical functions with identical doc comments — I diffed both bodies field by field (19 fields, same order). The drift claim also checks out: jobhash.Of (jobhash.go:23-49) omits Cities, EnglishLevel and IsTech while the document carries all three (jobview.go:67,77 and the fold at :154-180). Severity stays low, and one part of the narrative is wrong: a stale twin hash causes exactly ONE spurious `changed` on the next crawl (ingest recomputes and stores the full hash at cmd/ingest/store.go:87), not a re-push "on each pass".

#### ✅ 15. pipeline.Store's three optional capabilities are unexported interfaces discovered by runtime type assertion, so the sole production implementation cannot be checked at compile time

`pipeline-store-capabilities-unexported` · boundary · severity **low** · effort **S** · verdict **overstated**

**Problem.** The seam between the Runner and its store is one required method plus three capabilities matched at runtime, with a silent no-op on every miss. Because closer/toucher/seenLookup are unexported, cmd/ingest cannot state `var _ pipeline.toucher = (*dbStore)(nil)` the way it does for BoardHealth, so a signature drift on Touch, Close or ExistingExternalIDs compiles clean and degrades production quietly: a dropped Touch means a hydrating board's re-listed postings stop getting last_seen_at, and the 48h unseen sweep closes live jobs. The comments say the fallbacks exist "so test fakes are unaffected" — that is the whole justification for four interfaces where there is one implementation.

**Remedy.** Export the three interfaces (Closer/Toucher/SeenLookup) and add the three `var _` proofs in cmd/ingest beside the BoardHealth one. Drop the second alternative: folding Touch/Close/ExistingExternalIDs into Store contradicts the documented optional-capability design that ingestStream and fetchBoard branch on (pipeline.go:437,460,525) and forces every fake to grow three no-op methods — churn for the same guarantee the one-line export already buys.

**Evidence.**

- internal/pipeline/pipeline.go:33 — `Store` is a one-method interface (Save)
- internal/pipeline/pipeline.go:41, :51, :68 — `closer`, `toucher`, `seenLookup`: three more capabilities, all unexported
- internal/pipeline/pipeline.go:437 `r.Store.(seenLookup)`, :460 `r.Store.(toucher)`, :525 `r.Store.(closer)` — each miss silently degrades (list-only fetch, dropped liveness refresh, dropped removal)
- cmd/ingest/store.go:28 — dbStore, the only non-test implementation, satisfies all four; nothing asserts it, and nothing can, because three of the four interfaces are unexported outside the pipeline package
- cmd/ingest/board_health.go:24 — `var _ pipeline.BoardHealth = (*boardHealth)(nil)`: the one optional port that IS exported does get a compile-time proof, showing the intended discipline

**Verifier.** Citations are exact — Store at pipeline.go:33 (one method), closer:41, toucher:51, seenLookup:68, the three assertions at 437/460/525 each with a silent-degrade branch, and cmd/ingest/board_health.go:24 is indeed the lone `var _` proof. The asymmetry is real and the export is cheap. But the impact is inflated: this is a latent typing gap, not a defect — dbStore satisfies all four today, the optional-capability shape is explicitly documented at each interface AND in internal/pipeline/AGENTS.md:40 ("pipeline.ingestStream routes that to the Store's optional closer"), and there is no evidence of drift. "Degrades production quietly" describes a hypothetical future rename, which the rubric puts at low, not medium.

#### ✅ 16. cmd/backfill-derive hand-rolls its own db.Job → jobderive.Input mapping and drops the structured regions/cities, so a run permanently erases moderator-stated geography

`backfill-derive-rebuilds-derive-input` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** CLAUDE.md and the command's own header claim the backfill produces "byte-for-byte what a fresh ingest of the same raw fields would produce", but the ingest door is job.New (which cleans the location and accepts the structured signals) while the backfill calls jobderive.Derive directly with a shorter, hand-maintained field list. For a moderator- or submission-authored vacancy that stated Berlin/eu explicitly, one backfill-derive pass replaces those with whatever location.Parse makes of the free-text location — usually nothing — and nothing ever restores them, because the row is never re-crawled. Every future field added to jobderive.Input has the same failure mode: the backfill silently keeps deriving without it.

**Remedy.** Do not build a shared db.Job→jobderive.Input mapping for a single call site — there is no second site, so there is nothing to de-duplicate. Decide the underlying rule once and apply it in BOTH places that re-derive: either treat stated regions/cities as durable (skip those two columns for `created_by IS NOT NULL` rows in UpdateJobDerived, and stop blanking them in moderation.Update's `structuredFacets{}` call), or accept that they are a create-time-only hint and say so in the backfill header next to the grade/category/skills sentence.

**Evidence.**

- cmd/backfill-derive/main.go:146 — `jobderive.Derive(jobderive.Input{Title, Company, Source, ExternalID, Location, Description, WorkMode})`: seven fields, no Regions, no Cities, no Skills
- internal/moderation/moderation.go:253 — the moderator/submission path supplies `Regions: s.Regions, Cities: s.Cities, Skills: s.Skills` as authoritative structured signals
- internal/jobderive/jobderive.go:34 — "Regions and Cities are the caller's structured GEOGRAPHY signal … Unlike the scalars they fully replace the location-dictionary derivation for that facet when present"
- internal/db/queries/jobs.sql:911 (UpdateJobDerived) — the backfill's write statement overwrites `regions` and `cities` unconditionally (COALESCE to '{}')
- cmd/backfill-derive/main.go:33 — the header enumerates what is deliberately NOT preserved ("a grade, category, skills, or required-experience") and justifies it with "a boardless adapter like getmatch re-supplies the structured facets on its next full crawl"; regions/cities are absent from that list, and a manual job is never crawled at all
- internal/job/job.go:126 — job.New runs `normalize.CleanLocation` before Derive; the backfill's Input skips it, so the two doors are not the same door

**Verifier.** The factual core holds: cmd/backfill-derive/main.go:146 passes seven fields with no Regions/Cities, UpdateJobDerived (jobs.sql:911) writes `regions/cities = COALESCE(arg,'{}')` unconditionally, and backfillProgress pages the whole table with no source/created_by filter (cmd/backfill-derive/main.go:243-250) — so a pass does blank moderator-stated geography. But the framing fails on two counts. (1) The REUSE lens does not apply: deriveRow is the ONLY db.Job→jobderive.Input mapping in the repo (grep for `jobderive.Input{` returns 5 sites, the other four build an Input from raw adapter/moderator input, not from a row), so the two sides a reuse finding needs do not exist. (2) The erasure is not a backfill-specific divergence: internal/moderation/moderation.go:191-196 already documents "The edit path re-derives every facet from content and carries no explicit structured overrides" and passes `structuredFacets{}`, so ANY moderator edit — even a title typo fix — wipes the stated regions/cities through repository.Update (internal/moderation/repository.go:96-98) today. The project has already decided stored geography is re-derivable; the backfill is consistent with that, not a rogue second door. The CleanLocation point (job.go:126) is real but near-inert: every path that writes Location goes through job.New, so stored values are already cleaned and re-cleaning is idempotent.

### LLM / AI subsystem

> **Reviewer's note on what is well-factored here.** What is genuinely well-factored here, and worth protecting:\n\n- **`internal/llm` is a real single provider abstraction, and nothing reaches past it.** All eight LLM callers (`enrich`, `telegram`, `resumeextract`, `matchanalysis`, `atscheck`, `mailclassify`, `autofillagent`, `assistant/followups`) go through `GenerateJSON`/`GenerateJSONStream`/`Chat`. `llm.NewClient` (llm.go:141) is the only construction path and it wires tracing there, so no entrypoint can build a client and forget it. The one package with its own `http.Client` — `internal/speech` (speech.go:56) — is explicitly argued for in internal/speech/AGENTS.md (langchaingo models chat completions and has no audio surface), and the argument is right. `internal/llm/schema.go`'s `schemaInjector` is a genuinely well-reasoned workaround for a langchaingo type that cannot express `[\"string\",\"null\"]`, cached per rendered schema and keyed to defeat name collisions.\n\n- **The tolerant-decoder family is deliberate, not drift.** `internal/flexjson`'s package doc (flexjson.go:11-16) names its two siblings — `enrich`'s `roundInt`/`stringOrFirst` and `resumeextract`'s `truncInt`/`verbatimString` — and states the exact semantic each diverges on (round vs truncate, coerce vs keep verbatim, strict vs lenient). I looked for drift and found the opposite: three of the four packages that could reuse `flexjson` do (`matchanalysis/flexdecode.go`, `telegram/flexdecode.go`, `mailclassify/flexdecode.go`), and the two that don't say why. Not flagged.\n\n- **Prompts are owned.** Per-preset system prompts live in `internal/assistant/prompt.go` behind `NormalizePreset`, with `TestPromptOnlyNamesToolsThePresetHas` guarding prompt/registry agreement. Feature prompts sit in the package that owns the contract they describe (`enrich/langchain.go:83`, `matchanalysis/analyzer.go:224-293`, `telegram/llm.go:97`). The two server-side briefs in the handler (`openingBrief` assistant.go:486, `autopilotBrief` assistant.go:536) are turn kick-offs, not method — both say so and both point at the system prompt that holds the method. Correct.\n\n- **`internal/experience`'s split is the sharpest boundary in the area.** `professional.go:11-14` names itself as an adapter so `Store`/`Retrieve` stay ignorant of résumés; the publish gate is a service check (`experienceFromBank` skipping `!atom.Provenance.Publishable()` at professional.go:74), not a prompt rule.\n\nWhat I deliberately did NOT flag:\n\n- **`matchanalysis`, `atscheck` and `autofillagent` not passing `llm.WithSchema`.** It looks like an inconsistency (five of eight callers do), but openspec/changes/archive/2026-07-28-llm-structured-outputs/design.md:46 explicitly scoped them out and design.md:190 records the follow-up as open. A documented deferral, not a defect.\n\n- **The `sync.Once` + `schema` + `schemaErr` + `requestSchema()` idiom repeated in five packages** (`enrich/schema.go:49`, `resumeextract/schema.go:20`, `telegram/schema.go:14`, `mailclassify/schema.go:14`, `assistant/followups.go:96`). Real repetition, ~12 lines each, but the only honest fix is a generic memoizer in `llmschema` — premature generics for a per-process one-time derivation the client already caches downstream (llm/schema.go:119). Below the bar.\n\n- **`experience.ProfessionalFrom` (professional.go:58) is called only from `professional_test.go:39`** — a two-line duplicate of `Store.Professional`'s body with no production caller. Orphaned, but it is pre-existing and one line of cleanup, not a structural finding.\n\n- **`llm.TruncateRunes`/`TrimTruncateRunes` used by `internal/cv` and `internal/experience`,** which make no LLM calls. Mildly the wrong home, but two pure functions in an otherwise coherent package is naming taste, not structure.\n\n- **Assistant tool schemas hand-written as `map[string]any` in 26 places** rather than derived via `llmschema`. They are argument schemas for closures over anonymous structs, not contract types; `assistant.DecodeArgs` (tool.go:137) already rejects unknown fields and trailing content strictly. Deriving them would need a named struct per tool for no gain.

#### 17. The "de-identified CV" seam is a JSON string, so the contact whitelist is enforced three different ways — one of them the blacklist the codebase's own doc calls wrong

`cv-deidentification-seam-is-a-json-string` · boundary · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** Three call sites answer the same question — "what part of the candidate's CV may a model see?" — and only one of them is typed. Because `atscheck.Analyze` and `matchanalysis.Input.StructuredResume` both take a bare `string`, nothing stops a caller passing the contact-bearing `Structured`, and `ats_report.go:211` does exactly that. The safety net there is `stripContacts`, a blacklist over four hard-coded keys, which is precisely the mechanism `resumeextract/structured.go:62` argues against. Add a field to `Structured` (an `address`, a `github`, a `date_of_birth` from a European CV) and it ships to the gateway through the ATS review while `matchanalysis` correctly withholds it — a silent divergence in the one invariant both paths exist to hold. On the fit path the string typing also buys a pure-overhead unmarshal/re-marshal that only appears to de-identify: the input is already `Professional`.

**Remedy.** Same shape as proposed, plus one wrinkle the remedy skips: Analyze's "no CV" signal is currently the empty string. Make handler.structuredResumeJSON return (resumeextract.Professional, bool) — `st.Professional(), true` — and have PostATSReport skip the LLM call when !ok, rather than teaching atscheck to test a zero value. Then Analyze(ctx, cv resumeextract.Professional) marshals + truncates in reviewUserPrompt, stripContacts (analyzer.go:106) is deleted, Input.StructuredResume becomes resumeextract.Professional, and candidateContext (analyzer.go:380) collapses to marshal + TruncateRunes. No new package or interface.

**Evidence.**

- internal/resumeextract/structured.go:62 — Professional's doc: "The field set is a whitelist, deliberately... A blacklist — dropping the four known contact keys — would disclose that new field by default, which is the wrong way round"
- internal/atscheck/analyzer.go:106 — `stripContacts` does exactly that blacklist: `for _, k := range []string{"full_name", "email", "phone", "links"} { delete(m, k) }` over a `map[string]json.RawMessage`
- internal/handler/ats_report.go:211 — `blob, err := json.Marshal(st)` marshals the FULL `resumeextract.Structured` (contacts included) and ats_report.go:83 hands that string straight to `h.atsAnalyzer.Analyze`
- internal/handler/match_analysis.go:333 — the fit path marshals a `resumeextract.Professional` (already contact-free) into the same kind of string
- internal/matchanalysis/analyzer.go:380 — `candidateContext` unmarshals that string back into `resumeextract.Structured` and calls `.Professional()` again, re-marshalling: a full JSON round trip whose second projection is a no-op on the fit path
- internal/matchanalysis/analyzer.go:61 — `StructuredResume string` is the field type; internal/handler/me_profile.go:92 shows the correct shape of the same seam, returning `*resumeextract.Professional`

**Verifier.** Every citation is real and says what is claimed. internal/resumeextract/structured.go:57-64 states the whitelist rationale verbatim ("A blacklist — dropping the four known contact keys — would disclose that new field by default, which is the wrong way round"), and internal/atscheck/analyzer.go:106-125 is precisely that blacklist (`for _, k := range []string{"full_name","email","phone","links"} { delete(m, k) }`). internal/handler/ats_report.go:211 does `json.Marshal(st)` over the FULL resumeextract.Structured and ats_report.go:83 feeds it to Analyze, so the blacklist is the only thing standing between a new Structured field and the gateway. The complement of Professional today is exactly those four keys, so nothing leaks yet — but the two mechanisms are only accidentally equal. The fit path is as described: handler/match_analysis.go:321-337 marshals a resumeextract.Professional (candidateProfiler.Professional, match_analysis.go:356), and matchanalysis/analyzer.go:385-393 unmarshals it into Structured and re-projects — a genuine no-op round trip. matchanalysis/AGENTS.md calls Input.StructuredResume "the resumeextract wire shape (contact fields stripped)", i.e. the code enforces by convention what it documents as a type. I checked the deterministic ATS score (internal/atscheck/atscheck.go:126 Score(cvText, ...)) — it never reads Structured, so no consumer needs the contact fields, and only two producers exist (candidateProfileJSON ×2, structuredResumeJSON ×1), so the type swap is small. Downgrading to medium only because nothing leaks today: the defect fires the first time somebody adds a field to Structured, which is ordinary but future work, not an active bug.

#### 18. internal/handler carries two hand-rolled SSE stream writers with the same headers, deadline discipline and keepalive, and they already share one constant across the split

`two-sse-stream-implementations-in-handler` · reuse · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** Both long-lived SSE endpoints in the API had to solve the same four problems (fasthttp's WriteTimeout racing the stream writer, a cleared deadline pinning the goroutine forever, bufio.Writer not being concurrency-safe against the keepalive ticker, and the request-scoped Sentry hub dying before the writer runs). One solved it with a small type; the other copied the reasoning into an inline closure plus two package-level free functions. Drift has already started: the assistant's keepalive interval is the named `assistantKeepalive` (assistant.go:133) while the analysis stream uses a bare `15 * time.Second` literal (match_analysis_stream.go:125). The next SSE endpoint, or the next fix to the deadline dance, has to be applied twice and there is nothing that makes that obvious.

**Remedy.** Trim the factory. `w` only exists inside the StreamWriter callback, so an `openSSE` returning a stream factory buys nothing. Do just: give sseStream.event an `(bool)` return (true on marshal failure — an unencodable frame is our bug, not a dead client), rewrite streamTurn over newSSEStream(w, conn, sseWriteTimeout), delete writeEvent/writeComment (assistant.go:635,645) and the inline mu, and pass the keepalive interval as an argument. If the headers still itch, a four-line `sseHeaders(c *fiber.Ctx)` next to sseStream is enough; leave the hub clone inline where its comment lives.

**Evidence.**

- internal/handler/match_analysis_stream.go:202 — `sseStream` type owning `mu sync.Mutex`, `w *bufio.Writer`, `conn net.Conn`, `timeout`; `event`/`comment`/`write` at 214/224/231 do the framing, the per-write `SetWriteDeadline`, and the Flush
- internal/handler/assistant.go:411 — the identical machinery hand-rolled inline: `write := func(event string, data any) bool` at 419 setting `conn.SetWriteDeadline(time.Now().Add(sseWriteTimeout))`, a bare `var mu sync.Mutex` at 433, and a keepalive goroutine at 437 that locks/sets-deadline/writes just like match_analysis_stream.go:123
- internal/handler/assistant.go:645 — `writeEvent` formats `"event: %s\ndata: %s\n\n"`; internal/handler/match_analysis_stream.go:219 formats the same string. assistant.go:636 and match_analysis_stream.go:225 both format `": %s\n\n"`
- internal/handler/match_analysis_stream.go:190 — `sseWriteTimeout` is declared here and used from assistant.go:421 and 448, so the assistant already reaches into the other file's constants while re-implementing the type that owns it
- internal/handler/assistant.go:393 vs internal/handler/match_analysis_stream.go:77 — the same four `c.Set(...)` header lines, comment included; assistant.go:406 vs match_analysis_stream.go:90 — the same sentry hub clone with the same rationale comment

**Verifier.** Citations check out line for line. internal/handler/match_analysis_stream.go:202-241 is the sseStream type (mu/w/conn/timeout; event at 214, comment at 224, write at 231 doing the per-write SetWriteDeadline + Flush), and internal/handler/assistant.go:411-454 hand-rolls the same thing: the write closure at 419-424 setting the same deadline, `var mu sync.Mutex` at 433, and a keepalive goroutine at 437-454 that locks, re-arms the deadline and writes. The frame formats are identical (assistant.go:650 vs match_analysis_stream.go:219; assistant.go:636 vs 225), the four header Set calls are identical including the nginx comment (assistant.go:393-396 vs 77-80), and the hub clone with the same rationale is identical (assistant.go:406-409 vs 90-93). The cross-file reach is real: `sseWriteTimeout` is declared at match_analysis_stream.go:190 and used from assistant.go:421 and 448 (grep confirms these are the only two SetBodyStreamWriter sites in the repo). The duplicated part is not vendor-shaped divergence — it is one subtle concurrency/deadline protocol, documented in both files with near-identical prose, and the only real behavioural difference is one bool. Drift evidence beyond the keepalive literal: handler/AGENTS.md:91 still contrasts writeEvent with `writeSSE`, a function that no longer exists anywhere in the tree. Nothing in handler/AGENTS.md blesses the split. Medium stands: no live bug, but the next fix to the deadline race has to be made twice.

#### ✅ 19. Two config structs declare the same six LLM/Langfuse fields and seven entrypoints hand-copy them into llm.Settings field by field

`llm-settings-mapped-by-hand-seven-times` · simplicity · severity **low** · effort **S** · verdict **confirmed**

**Problem.** `llm.Settings` was introduced to be the single shape, and it is — but the mapping INTO it is copied seven times over two duplicate config structs that read the same six environment variables. A seventh field (a per-source budget, an org header, a second gateway) means editing nine places, and a new LLM-using binary starts by copy-pasting a literal rather than by naming a config field. The asymmetry is already visible: `Enrich.LangfuseEnabled` has no counterpart on `Config`, so the server's tracing predicate lives implicitly inside `llm.NewClient` while the workers' lives in config.

**Remedy.** Skip the parallel `config.LLM` struct and the `llm.Settings(cfg.LLM)` conversion — that trick silently depends on field ORDER matching llm.Settings. internal/llm imports nothing from internal/config, so just give both structs a named field `LLM llm.Settings` filled by one `loadLLMSettings()`, and every call site becomes `llm.NewClient(cfg.LLM, "enrich")`; cmd/server's assistant client copies and overrides Model with cmp.Or(cfg.AssistantModel, cfg.LLM.Model). Delete Enrich.LangfuseEnabled and its test rather than moving dead code onto the new field; llm.Settings.Enabled already covers the live predicate.

**Evidence.**

- internal/config/config.go:70 declares `LLMBaseURL/LLMAPIKey/LLMModel`, config.go:99 declares `LangfuseBaseURL/LangfusePublicKey/LangfuseSecretKey`, and config.go:192-205 reads the six env vars
- internal/config/enrich.go:15 and enrich.go:26 declare the same six fields; enrich.go:42-51 reads the same six env vars again
- internal/config/enrich.go:34 — `func (e Enrich) LangfuseEnabled() bool` exists only on `Enrich`; `Config` carries the same three fields with no such method
- cmd/server/main.go:143 and cmd/server/main.go:160 — two six-line `llm.Settings{BaseURL: cfg.LLMBaseURL, APIKey: ..., LangfuseSecretKey: cfg.LangfuseSecretKey}` literals
- cmd/enrich/main.go:34, cmd/tg-extract/main.go:46, cmd/classify-mail/main.go:32, cmd/backfill-experience/main.go:205, cmd/backfill-resume-structured/main.go:70 — five more of the identical literal, differing only in the source struct and the trailing source label
- internal/llm/llm.go:117 — `Settings` already documents itself as "the one shape every entrypoint maps its env config into, so client construction and tracing live in exactly one place"

**Verifier.** The count is exact: seven llm.Settings literals with the same six fields — cmd/server/main.go:143 and :160, cmd/enrich/main.go:34, cmd/tg-extract/main.go:46, cmd/classify-mail/main.go:32, cmd/backfill-experience/main.go:205, cmd/backfill-resume-structured/main.go:70 — over two config structs that read the same six env vars (config.go:70-72 + :99-101, read at :192-205; enrich.go:15-17 + :26-28, read at :42-51). llm.go:117-120 does claim to be "the one shape every entrypoint maps its env config into", and the mapping is what got copied. One supporting claim is wrong, though: Enrich.LangfuseEnabled (enrich.go:34) is referenced by nothing outside config/enrich_test.go — grep shows zero production callers — so it is dead code, not an asymmetry between the server's and the workers' tracing predicate; both actually get their predicate from llm.NewTracer inside llm.NewClient (llm.go:150). Severity low is right: forgetting a site when a seventh field lands silently disables that binary's tracing, which is annoying, not dangerous.

#### ✅ 20. The outbox lease/attempt/dead-letter policy is written three times in SQL and three times in Go, and the third copy silently dropped the dead-letter signal

`outbox-drain-hand-rolled-three-times` · reuse · severity **low** · effort **L** · verdict **overstated**

**Problem.** Three queues (`enrichment_outbox`, `semantic_outbox`, `email_classification_outbox`) each got their own claim CTE, their own failure-recording statement, their own `Store` port and their own drain loop. The claim predicate and the `attempts + 1 >= max_attempts THEN now()` dead-letter rule are one policy expressed six times. The copy has already lost information: `FailEmailClassification` is `:exec` where its two siblings are `:one ... RETURNING`, so `maillink.Runner` cannot know an entry dead-lettered, keeps no tallies, and `cmd/classify-mail` exits 0 no matter how much mail permanently failed — while `cmd/enrich` and `cmd/embed` both surface it to cron through `worker.ExitCode`. A queue that quietly stops working is exactly what that exit code exists to catch.

**Remedy.** Drop the worker.Outcome extraction; leave enrich's and embed's failN alone. Make FailEmailClassification `:one ... RETURNING attempts, failed_at` to match its two siblings, widen maillink.Store.Fail to `(deadLettered bool, err error)`, have maillink.Runner.Run return a small `Stats{Failed, DeadLettered}` tallied where it already logs the bookkeeping error (runner.go:133-136), and end cmd/classify-mail on worker.ExitCode(stats.Failed, stats.DeadLettered) like every other worker.

**Evidence.**

- internal/enrich/runner.go:26 — `Store` with `Enqueue`/`Claim`/`Fail(...) (deadLettered bool, err error)`; Run loop at 67-98; `fail`/`failN` tallying `Failed`/`DeadLettered` at 188-212
- internal/embed/runner.go:36 — the same `Store` shape and the same `Fail(...) (deadLettered bool, err error)`; Run loop at 98-127; `fail`/`failN` at 257-275, whose comment at 262 says "mirrors enrich"
- internal/maillink/runner.go:62 — the third copy: `EnqueuePending`/`ClaimBatch`/`Fail(ctx, outboxID, cause, maxAttempts) error` — no dead-letter bool. Run loop at 120-144 keeps no Stats at all
- internal/db/queries/enrichment.sql:53 — `RecordEnrichmentFailure :one ... RETURNING attempts, failed_at`; internal/db/queries/semantic.sql:113 — `RecordSemanticFailure :one`, its comment saying "Mirrors RecordEnrichmentFailure"
- internal/db/queries/mail_classification.sql:103 — `FailEmailClassification :exec` with the identical body but no RETURNING, its comment conceding "Mirrors RecordEnrichmentFailure / RecordSemanticFailure"
- cmd/enrich/main.go:70 logs `dead_lettered=%d` and returns `worker.ExitCode(stats.Failed, stats.DeadLettered)`; cmd/embed/main.go:83 does the same; cmd/classify-mail/main.go:59 logs `classify-mail: done` and returns 0 unconditionally
- internal/db/queries/enrichment.sql:30 vs semantic.sql:41 vs mail_classification.sql:20 — the same `WITH claimable AS (... failed_at IS NULL AND (claimed_at IS NULL OR claimed_at < now() - make_interval(secs => lease_seconds)) ... FOR UPDATE OF o SKIP LOCKED LIMIT batch_size)` CTE, copied verbatim

**Verifier.** The citations are accurate but the framing inflates a small, real drift into triplicated policy. The SQL side is not a defect: sqlc has no way to express one claim/fail statement over three different tables, so enrichment.sql:30, semantic.sql:41 and mail_classification.sql:20 must be three statements — and the finding's own remedy concedes this. The Go side is two ~15-line failN funcs (enrich/runner.go:194-212 vs embed/runner.go:262-275) that already differ for real reasons: enrich tallies under a mutex because a wave runs concurrently, embed does not and deliberately calls Fail on context.Background() so a timed-out call still records itself. Extracting a worker.Outcome + a Fail port shape for two (soon three) callers of a dozen lines is symmetry-driven abstraction, and internal/worker is a good place to put it only if there were more. What IS confirmed, and is the only part worth acting on: internal/db/queries/mail_classification.sql:103 is `FailEmailClassification :exec` where its two siblings are `:one ... RETURNING attempts, failed_at`; internal/maillink/runner.go:69 declares `Fail(...) error`; the Run loop at 131-138 keeps no tallies; and cmd/classify-mail/main.go:59-60 logs "done" and returns 0 unconditionally. worker.ExitCode is a live convention in eight binaries (cmd/enrich:72, cmd/embed:85, cmd/notify:77, cmd/ingest:231, cmd/liveness:182, cmd/tg-ingest:56, cmd/remind:69, cmd/tg-extract:83) — classify-mail is the lone bypass, and nothing else reads email_classification_outbox.failed_at, so a permanently dead-lettering mail queue is visible in journalctl only. Low rather than medium: no user-visible bug, and the fix is roughly ten lines.

### CV / user-profile domain

> **Reviewer's note on what is well-factored here.** What is genuinely well-factored here, and worth protecting:

- **The single-writer claim holds at the SQL level.** `UpdateCV` (internal/db/queries/cvs.sql:52) has exactly one caller, `cvedit/repository.go:89`, inside the `GetCVForEdit … FOR UPDATE` transaction. `cv.Repository` (internal/cv/store.go:76) deliberately declares no document-writing method and says so. The two other `cvs`-row writers (`SetCVTracerLinks`, `SetCVAutopilotReport`) are columns outside `cvedit.State` with a stated reason each. Creation (`CreateCV`/`CreateTailoredCV`) never accepts a client-supplied document — internal/handler/cv.go:296 builds it from `EmptyDocument()` or `cv.Seed`.
- **The evidence gate really is in the service path**, not a prompt: `Editor.authorize` runs before the transaction opens (internal/cvedit/editor.go:145), `CommitDocument` re-derives ops and authorizes them under the lock (editor.go:337), and `assistant/prompt.go` only *restates* what the code enforces. My finding above is about how the gate is wired, not about where it lives.
- **`cvedit/path.go`'s reflection-derived vocabulary** is the right answer to the drift problem, and `Describe`'s `humanise` fallback (describe.go:75) means an unnamed new field degrades to a sentence rather than a blank feed line.
- **`renderedCVText`** (internal/handler/cv_rendered_text.go:28) is a real shared chokepoint: the ATS delta and the job-match score cannot disagree about what the document says.

**On the four-packages question the brief asked about:** the `cv` / `resume` / `resumeextract` / `cvsection` seam is earned, not historical. `cv` owns the structured document the user builds and renders; `resume` owns the uploaded *file* plus its S3 pointer and staleness stamps; `resumeextract` is a self-contained prompt unit over that file; `cvsection` is a pure, I/O-free text segmenter for the verdict's declared-vs-hidden skill split. Four distinct lifecycles, four distinct dependency sets. `cvsection`'s name is misleading (it segments an uploaded résumé's text, never a `cv.Document`) but that is naming, which this review is not about.

**What I deliberately did NOT flag:** `internal/headshot` and `internal/resume` are near-identical blob+pointer stores (Enabled/Put/Get/Status/Delete, nil-degradation). Unifying them would be exactly the speculative infrastructure CLAUDE.md warns against — their payloads, their validation, and their extra columns genuinely differ. Also skipped: `accountdelete`'s ordering (Postgres/S3/Google), which is correct and well-documented; and the `ListUserBlobKeys` SQL enumeration, which is a load-bearing list but is the right place for it.

#### 21. The evidence gate — the rule the tailoring capability exists to enforce — is an optional post-construction mutator with two independent fail-open nil checks

`evidence-gate-is-optional-by-construction` · coupling · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** Reordering handler assembly, or building the CV handlers in any context that does not also build the assistant, silently disables the honest wall: an API-key holder editing as `ActorAgent` through `PATCH /me/cvs/:id` writes arbitrary claims into a CV with no citation, no compile error, and no test failure. The dependency is security-critical but the type system models it as optional, and the nil check is duplicated so even a correctly-wired-but-bank-less `bankGate` passes everything. The doc's own justification for the mutator (the bank is constructed later) is false in the current wiring.

**Remedy.** Hoist the bank to one variable before internal/handler/handler.go:289, pass it into newCVHandlers, and construct the editor with the real gate at cv.go:92. Delete Editor.WithEvidenceGate and the `g.bank == nil` branch in bankGate.Publishable. Stop there — do NOT add a cvedit.NoGate{} type or make NewEditor reject nil: requireEvidence already short-circuits for ActorCandidate (policy.go:161-163), so a nil gate is legitimate for the eleven candidate-only test editors, and forcing a stub on all of them is churn for no invariant.

**Evidence.**

- internal/handler/cv.go:92 — the editor every CV write goes through is built with an explicitly nil gate: `cvedit.NewEditor(cvedit.NewRepository(pool, queries), nil)`
- internal/cvedit/policy.go:156 — `requireEvidence` opens with `if e.gate == nil { return nil }`, so a nil gate is silently "no evidence required"
- internal/handler/assistant_cv_tools.go:356 — `bankGate.Publishable` opens with `if g.bank == nil { return nil }`, a second independent fail-open on the same rule
- internal/handler/assistant.go:95 — the ONLY production wiring is inside `newAssistantHandlers`: `if cvH != nil && cvH.editor != nil { cvH.editor.WithEvidenceGate(bankGate{bank: h.experience}) }`
- internal/handler/cv_tailor.go:241 — `PatchCV` (`PATCH /me/cvs/:id`, mounted `mw.key` at internal/handler/cv.go:157) sets `actor = cvedit.ActorAgent` for any API-key caller; that agent write path has nothing to do with the assistant, yet depends on the assistant's constructor having run
- internal/handler/handler.go:266 — the bank is in fact already constructible before the CV handlers (`experience.NewStore(experience.NewQueriesRepository(queries))` at line 266, `newCVHandlers` at line 289), so the stated reason for late binding ("the bank is wired later than the CV handlers", internal/cvedit/editor.go:129) does not hold
- internal/handler/assistant_integration_test.go:80 — every test harness must remember `h.cv.editor.WithEvidenceGate(...)` by hand, with a comment admitting "Without it here the run could write an unevidenced claim and the test that says it cannot would pass for the wrong reason"

**Verifier.** Every citation is accurate. internal/handler/cv.go:92 builds the editor with an explicit nil gate; internal/cvedit/policy.go:157 `if e.gate == nil { return nil }`; internal/handler/assistant_cv_tools.go:355-357 `if g.bank == nil { return nil }` is a second, independent fail-open on the same rule (and g.bank is never nil in production — it is always experience.NewStore at internal/handler/assistant.go:75); the only production attachment is internal/handler/assistant.go:96. The doc's stated reason at internal/cvedit/editor.go:128-131 does not hold: an equivalent experience.Store is already constructed at internal/handler/handler.go:266, twenty-three lines before newCVHandlers at handler.go:289, and the store is stateless. The agent write path is genuinely independent of the assistant — internal/handler/cv_tailor.go:241-243 sets ActorAgent for any API-key caller on PATCH /me/cvs/:id, mounted mw.key at internal/handler/cv.go:157. cvedit's own comment calls a missing gate 'a wiring mistake rather than a configuration choice', and internal/handler/assistant_integration_test.go:67-80 admits the hazard in a comment while re-doing the wiring by hand. Not 'high' though: production is wired correctly today, so nothing is broken now and no feature work is blocked — this is a latent fail-open, not a live bug.

#### 22. experience.ProfessionalFrom is a second implementation of Store.Professional reachable from nothing but its own tests, and the docs still name it as the fit-analysis path

`professionalfrom-orphaned-duplicate` · simplicity · severity **low** · effort **S** · verdict **confirmed**

**Problem.** An exported function with a long rationale comment and seven dedicated tests, that no production code path calls. It duplicates `Store.Professional`'s rule, so a future change to the composition has two places to land and one of them is only exercised by tests that will keep passing. The AGENTS.md that a reader consults first points at the dead one, so anyone tracing "what does the fit chain actually send" is sent to the wrong function.

**Remedy.** Delete `ProfessionalFrom` and point its tests at `Store.Professional` (or at `experienceFromBank`, which is the part they actually exercise). Update internal/resumeextract/AGENTS.md:20 to name `experience.Store.Professional`.

**Evidence.**

- internal/experience/professional.go:58 — `func ProfessionalFrom(st resumeextract.Structured, employments []Employment, atoms []Atom) resumeextract.Professional` with a 16-line doc comment defending the design
- internal/experience/professional.go:18 — `Store.Professional` does the same composition (`out := st.Professional(); out.Experience = history`) and is what every real caller uses
- internal/handler/match_analysis.go:321 — the fit chain calls `bank.Professional(...)`, not `ProfessionalFrom`
- internal/resumeextract/AGENTS.md:20 — still claims "`experience.ProfessionalFrom` takes the work history from the bank and everything else from here, and that is the only candidate text `matchanalysis` sends"
- internal/experience/professional_test.go:39 — the only non-test-file references are in professional_test.go (7 call sites), so nothing in the binary reaches it

**Verifier.** Verified by exhaustive grep over the repo (excluding .claude worktrees): the only references to ProfessionalFrom are internal/experience/professional.go:42/58 and seven call sites in internal/experience/professional_test.go. No cmd/ or internal/ non-test caller exists. It duplicates the two-line composition of Store.Professional (internal/experience/professional.go:23-24 vs :59-60), and the real fit path is internal/handler/match_analysis.go:321 `bank.Professional(...)`. internal/resumeextract/AGENTS.md:20 does still name `experience.ProfessionalFrom` as 'the only candidate text matchanalysis sends', which sends a reader to the dead function. Remedy is proportionate: delete, retarget the tests at experienceFromBank/Store.Professional, fix the doc line.

#### 23. "Work history comes from the bank, the rest from the structure, and the structure's own experience is discarded" is reimplemented in four handlers instead of once in internal/experience

`bank-plus-structure-composition-inlined-four-times` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** One domain rule with four inline copies, and they have already drifted in their degradation: with a nil/failing bank, resume.go serves the structure with an empty work history, cv_seed.go seeds a CV from education alone, me_profile.go returns the structure's Professional with Experience nil, and match_analysis.go returns "" and refuses to analyse. A change to the rule (say, merging placeless atoms differently, or taking the summary from the bank too) has to be found in four handlers, one of which owns no CV concern at all.

**Remedy.** Not worth a new domain method on its own. If it is done, do it together with making the bank non-nil at construction (see finding 5's premise): only then does resume.go/cv_seed.go collapse to a single `st, err = bank.WithWorkHistory(ctx, userID, st)` and the discard move inside internal/experience where it belongs. Otherwise leave the two six-line blocks alone.

**Evidence.**

- internal/handler/resume.go:400 — `st.Experience = nil` then `if h.bank != nil { history, err := h.bank.WorkHistory(...); ... st.Experience = history }` (lines 393-408)
- internal/handler/cv_seed.go:53 — `st.Experience = nil` then `if s.bank != nil { history, err := s.bank.WorkHistory(...); ... st.Experience = history }` (lines 44-61) — the same twelve lines, same log-and-continue
- internal/handler/me_profile.go:106 — a third form: `professional := structured.Professional(); professional.Experience = nil; if h.bank != nil { composed, err := h.bank.Professional(...) }` (lines 96-115)
- internal/handler/match_analysis.go:317 — a fourth: read `h.resume.Structured` best-effort, then `bank.Professional(ctx, userID, st)` (lines 307-329)
- internal/experience/professional.go:18 — the domain already owns the rule (`Store.Professional`), but only in the contact-free shape, so the two callers that need the contact-bearing `Structured` reimplement it
- internal/experience/AGENTS.md:73 — the boundary table states this rule as the package's own: employments/atoms belong to the bank, everything else to `resumeextract`

**Verifier.** The four sites exist, but two of them are not copies of the rule — internal/handler/me_profile.go:108 and internal/handler/match_analysis.go:321 both CALL the domain method experience.Store.Professional (internal/experience/professional.go:18). Only internal/handler/resume.go:400-408 and internal/handler/cv_seed.go:53-61 inline it, so it is two sites and about six lines, not four handlers. The 'they have already drifted in their degradation' framing is also wrong: each degradation is a documented, deliberately different policy (resume.go:385-391, cv_seed.go:51-52, me_profile.go:103-105, match_analysis.go:327-331), and the finding itself later concedes this. Worse, the proposed remedy does not remove what it identifies: the rule that must not drift is 'the structure's own experience is discarded', which lives in the `st.Experience = nil` line, and with a possibly-nil bank interface that line plus the nil-check plus the log-and-continue all still have to stay at each caller — WithWorkHistory would save roughly one line per site.

#### ✅ 24. "Which CV fields are the candidate's identity" is answered by three hand-maintained lists in two packages, and the read-side one is a blacklist

`contact-field-classification-in-three-hand-lists` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** Adding one field to `cv.Header` (a portfolio URL, a second phone, a website) fans out four ways with no compiler help. `Paths()` publishes `header.website` to the model's tool schema immediately. `DefaultPolicy` does not deny it, so the agent may write it. `presentationShapes` does not list it, so the agent is asked for an `evidence_id` for a URL — a demand the bank can never satisfy. And `withoutContacts()` does not clear it, so it leaks into `cv_get` and into the tailoring prompt. The one list that is derived from the struct (Paths) opens the door, while the three that would close it are hand-typed. resumeextract already argued this exact case and chose a whitelist; the CV side, which is the surface that actually reaches a model with the candidate's real contact block, chose the blacklist.

**Remedy.** One line, no reflection and no tags: make internal/cv/cv.go:179 build the redacted header instead of zeroing four fields — `d.Header = Header{Location: d.Header.Location}` — so a field added to Header is withheld by default, matching the direction internal/resumeextract/structured.go:57 already states. Leave presentationShapes alone (it already defaults to 'claim'). If the policy list is also worth deduplicating later, export the four contact paths as a single `[]string` from internal/cvedit and reference it from DefaultPolicy — do not invent struct tags for a five-field struct.

**Evidence.**

- internal/cv/cv.go:179 — `withoutContacts()` clears exactly four named Header fields (FullName, Email, Phone, Links); it is the ONLY redaction on `Store.GetForModel` (internal/cv/store.go:170), which feeds `cv_get` and the API-key `GET /me/cvs/:id`
- internal/cvedit/policy.go:36 — `DefaultPolicy()` denies the agent the same four addresses, restated as path strings: `header.full_name`, `header.email`, `header.phone`, `header.links`
- internal/cvedit/policy.go:121 — `presentationShapes` restates the header a third time (`header`, `header.full_name`, `header.email`, `header.phone`, `header.location`, `header.links`, `header.links[]`) to exempt it from the evidence gate
- internal/cvedit/path.go:221 — `Paths()` enumerates the addressable vocabulary by REFLECTION over `State`, so a new `cv.Header` field is published to the model's schema the moment it is declared, with no list to edit
- internal/resumeextract/structured.go:62 — the sibling projection states the opposite rule explicitly: "The field set is a whitelist, deliberately… A blacklist — dropping the four known contact keys — would disclose that new field by default, which is the wrong way round"
- internal/cvedit/path.go:47 — cvedit's own doc comment argues against exactly this shape: "Validation is by reflection over the json tags rather than against a list kept beside them. A hand-maintained vocabulary drifts"

**Verifier.** Citations are real: internal/cv/cv.go:179 zeroes exactly FullName/Email/Phone/Links; internal/cv/store.go:165 GetForModel applies it as the only redaction, reached from internal/handler/cv.go:331 (API-key GET) and internal/handler/assistant_cv_tools.go:216 (cv_get); internal/cvedit/policy.go:33-40 denies the same four paths; internal/cvedit/policy.go:106-127 is presentationShapes; internal/cvedit/path.go:221 Paths() is reflection-derived; internal/resumeextract/structured.go:57-65 does argue the whitelist direction. But 'three lists answering one question' is wrong. Only TWO sets coincide (withoutContacts and DefaultPolicy's header entries). presentationShapes answers a different question and gives a different answer — it exempts header.location, the bare `header` container and `header.links[]`, plus style/margins/title/template_id — and its own comment (policy.go:96-105) states that the inverted direction is deliberate so an unknown field defaults to 'is a claim', i.e. it already fails SAFE for a new field; being asked for an evidence_id is a refusal, not a leak. The remedy is also partly infeasible as written: Title and TemplateID are fields of cvedit.State (internal/cvedit/path.go:23-27), not of cv.Document (internal/cv/cv.go:61-75), so a `cv.PresentationPaths()` cannot own them. And the trigger is hypothetical — cv.Header has not changed since it was introduced (git log -L on internal/cv/cv.go:164-171 shows only the introducing commit #802).

### Mail, applications and notifications

> **Reviewer's note on what is well-factored here.** Genuinely well-factored, and deliberately not flagged:

- **The five mail packages are a real pipeline, not a chain.** No step calls the next: `gmailsync`/`mailingest` write `emails` and stop; `maillink` is driven by `email_classification_outbox` and never knows who wrote the row; `mailmatch` and `mailclassify` are pure and know nothing about persistence. The only cross-step coupling is the `Learner` port (maillink/runner.go:82-84) wired in cmd — an optional, nil-safe feedback edge. The deterministic-tier-can-auto-link / LLM-can-only-suggest asymmetry is enforced by one predicate with a comment explaining why it is one (maillink/decide.go:25-28).
- **`internal/inbox` is the right seam and earns its existence.** Two readers with nothing in common (Fiber handlers, in-process assistant tools) go through it, and `Queries` deliberately omits any mark-read method (inbox.go:32-37) so `Search` cannot mark even by accident — a contract enforced by the type system rather than by a comment, which is the standard the rest of this area should be held to.
- **`appevent` as a db-free vocabulary** with `SourceForMail` erroring rather than defaulting (appevent.go:80-91) is exactly right, and `TrustedForDayMath` puts the "only an employer set this timestamp" rule in one place.
- **`notify`'s two-stage MATCH/DELIVER ledger** with the PK doing the idempotency, the lease doubling as the crash reaper, and soft-skip separated from dead-letter is solid; my finding is about its *duplication* in `reminder`, not about the design.

Deliberately NOT flagged:
- `handler/inbox_harness.go:62-98` writing `emails` directly instead of through `inbox.Service` — it is one sqlc upsert with no rule attached; the rules (`external` is never classified, content-only refresh) are in the SQL where the enqueue predicate can see them.
- `internal/reminder` holding both the HTTP use case and the firing engine in one package — documented at reminder.go:5-8 and the concept is small; splitting it would buy nothing today.
- `inbox` importing `maillink` only for `ReadableBody` (maillink/body.go:17) — an odd direction (use case → worker), but the alternative is a fourth tiny package, and the doc's reason for sharing it is sound.
- `mailbox`'s package doc claiming "allocation lives in the handler" (mailbox.go:1-3, echoed at docs/agents/mail-stack.md:38) while `allocate.go` is in the package — stale prose, not a structural defect; the code is better than the doc.
- The three `emails` insert statements (gmail DO NOTHING / hosted DO NOTHING without thread_id / external DO UPDATE) — they differ in ways that matter, and merging them would need a flag argument per difference.

#### 25. The application pipeline's forward order and its terminal set live in internal/mailclassify, not in internal/userjob which owns the stage vocabulary

`stage-pipeline-order-owned-by-mailclassify` · boundary · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** "How an application may move through its stages" is a tracking-domain rule, and it is decided by the mail-classification package. `internal/inbox` and `internal/maillink` both reach into `mailclassify` for it, so the mail vocabulary and the application state machine are welded together — which the same file's comment (classification.go:13-15) explicitly argues against for the *event* vocabulary. The one-directional test lets a real drift through: insert a stage into `userjob.Stages` (say `take_home` between screening and interview) and `stageOrder` gives it rank 0, which ranks below `applied` and is not in `terminalStages`, so any forward signal "advances" an application out of it. `userjob/silence.go`'s hand-listed five active stages would go stale the same way.

**Remedy.** As proposed, and it stays small: move rank + terminal into internal/userjob (`IsTerminal`, `Forward(current, target) bool`) and leave `signalStage` — genuinely mail's own — in mailclassify, so AdvanceStage becomes signal→stage plus userjob.Forward. Note that rank cannot simply be the index in Stages: accepted/rejected/withdrawn sit AFTER offer there, so they would rank above the active stages; keep the explicit active-rank table inside userjob next to silenceThresholds (both already key on the same five stages) and make IsTerminal the guard. Then extend classification_test.go into the missing direction: every stage in userjob.Stages is either ranked or terminal.

**Evidence.**

- internal/mailclassify/classification.go:86-88 `stageOrder` — a second, independent encoding of the pipeline rank (applied 1 … offer 5).
- internal/mailclassify/classification.go:94-96 `terminalStages` — the settled-outcome set, defined in the mail package.
- internal/mailclassify/classification.go:114-126 `AdvanceStage(current, sig)` — the "strictly forward, never resurrect a settled application" rule.
- internal/userjob/stages.go:5-11 — `Stages` is documented as "the ordered application-stage vocabulary (active stages then terminal) and the single source of truth"; that ordering reaches the SPA via cmd/gen-contracts/main.go:285 → web/src/lib/generated/contracts.ts:1014.
- internal/userjob/silence.go:44-50 — a third table keyed by the same stage strings, listing exactly the five non-terminal stages by hand.
- internal/inbox/mutate.go:226 and internal/maillink/decide.go:51 — both the interactive path and the worker path ask `mailclassify.AdvanceStage` how an application may move.
- internal/mailclassify/classification_test.go:12-23 — the only binding between the two packages, and it checks one direction only (every mailclassify stage is a valid userjob stage), not that every userjob stage has a rank.

**Verifier.** Verified: mailclassify/classification.go:86-88 (stageOrder 1..5), :94-96 (terminalStages), :114-126 (AdvanceStage) — the forward-only, never-resurrect rule and the terminal set both live in the mail package, while userjob/stages.go:5-11 documents Stages as 'the ordered application-stage vocabulary (active stages then terminal) and the single source of truth' and feeds the SPA through cmd/gen-contracts/main.go:285 → contracts.ts:1014 (STAGE_VALUES verified). The latent trap is real: inserting a stage into userjob.Stages gives it stageOrder rank 0, below `applied`, and it is not in terminalStages, so AdvanceStage(current=new_stage, any forward signal) advances BACKWARD to `applied` — and classification_test.go:12-23 cannot catch it, since it only asserts mailclassify→userjob, never userjob→mailclassify. userjob/silence.go:44-50 hand-lists the same five active stages and would go silently stale too (as would userjob/buckets.go:45-49, a fourth stage-keyed table). Both consumers cited are accurate: inbox/mutate.go:226 and maillink/decide.go:51. One citation is misattributed: the 'don't fold two vocabularies together' argument is at appevent/appevent.go:12-15, not mailclassify/classification.go:13-15 (which are just signal constants) — it does not change the finding.

#### 26. The employer-reply ledger reconcile — a two-statement ordering rule — is implemented once in internal/inbox and again in cmd/classify-mail, so the worker's copy of the rule sits in a main package

`ledger-reconcile-lives-in-a-main-package` · boundary · severity **low** · effort **S** · verdict **confirmed**

**Problem.** A rule the mail-stack doc treats as the ledger's load-bearing invariant (retract before insert, because data-modifying CTEs would read the same pre-statement snapshot) exists as two copies, one of which is in a `cmd/` main and therefore outside every domain package's test surface and outside anything an `internal/` reader would grep. Adding a third statement to the reconcile, or changing the order, requires finding both. The classification worker — the highest-volume writer of `employer_reply` events — is the copy that lives furthest from the rule's documentation.

**Remedy.** Fine as proposed, but keep it a plain function, not a restructure: `func ReconcileMailEvent(ctx, q EventRecorder, userID, emailID int64, mailSource string) error` in internal/inbox, with EventRecorder being just the two methods already listed at inbox.go:52-53. Note the two callers must keep different error semantics — inbox.syncLedger stays best-effort/logged (the mutation already committed), the worker propagates the error inside its transaction — so the shared function returns error and each caller decides. If importing internal/inbox from cmd/classify-mail feels wrong, the alternative is equally cheap: leave the code and fix the two comments (mail-stack.md:107-119, store.go:138-139, appevent.go:2-3) to say there are two implementations of one ordering.

**Evidence.**

- internal/inbox/mutate.go:194-213 `syncLedger` — resolves `appevent.SourceForMail`, then `RetractSupersededEmailEvent` then `RecordEmailApplicationEvent`, with the comment "Order matters: the retraction must land before the insert".
- cmd/classify-mail/store.go:140-153 — the identical three steps, same order, same comment, inside the worker's private `dbStore.Save`.
- docs/agents/mail-stack.md:107-119 — states the rule as "one reconcile with five callers" and that "the reconcile is deliberately two statements in order"; in code it is two independent implementations of that ordering.
- internal/appevent/appevent.go:2-3 — package doc names "internal/maillink, internal/inbox, internal/jobtracking" as the paths that record into the ledger, but `internal/maillink` imports nothing from appevent (its ledger step happens in cmd/classify-mail/store.go).
- internal/inbox/inbox.go:52-53 — the two reconcile methods are already isolated as a narrow interface pair on `inbox.Queries`, which `*db.Queries` and a `WithTx` copy both satisfy.

**Verifier.** Both sites are exactly as described: inbox/mutate.go:194-213 (SourceForMail → RetractSupersededEmailEvent → RecordEmailApplicationEvent, with 'Order matters' at :200-201) and cmd/classify-mail/store.go:140-153 (same three steps, same order). The doc-vs-code contradiction is real and unusually blunt: mail-stack.md:107-119 says 'the rule is one reconcile with five callers' and store.go:138-139 asserts 'The reconcile is the same statement the inbox's manual paths call; the rule has one home' — there are two homes. appevent/appevent.go:2-3 naming internal/maillink as a recording path is also wrong; `grep -rn appevent internal/maillink/*.go` returns nothing. Two mitigations keep this small: the ordering is pinned at the query itself (application_events.sql labels them 'Step 1'/'Step 2' and RecordEmailApplicationEvent's comment says 'Run RetractSupersededEmailEvent first'), and `grep RetractSupersededEmailEvent` finds both callers, so the rule is not as unfindable as claimed. Actual duplicated logic is ~10 lines.

#### ✅ 27. "When did this application last move" and "is a suggestion still pending" are copied into four SQL statements whose comments swear they mirror each other — and the pending-suggestion copies already test different columns

`application-activity-rule-copied-into-four-queries` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** One business rule, four hand-maintained copies, kept in step by prose. The copies have already diverged: the board's pending-suggestion test asks whether the message has a `job_id`, the follow-up gate's and the ghost signal's ask whether it has an `application_id`, and those are not the same set — `application_id` is deliberately NULL whenever mail is linked to a posting the caller has no `applications` row for. A message that carries a stale suggestion while linked to such a posting therefore reads "no pending suggestion" on the board (badge `silent`, follow-up button offered) and "pending suggestion" at the gate (`unconfirmed`, 409). That is precisely the disagreement mail_linking.sql:3-5 declares impossible.

**Remedy.** Skip the view. Make user_jobs.sql:301 test `e.application_id IS NULL` like mail_linking.sql:23, ghost.sql:38 and the inbox link filter (gmail.sql:124,151,230) already do — one line, and it settles which column means 'confirmed' before the two spellings ever can diverge. Consider the shared view only if a fifth copy of the last-activity expression appears.

**Evidence.**

- internal/db/queries/user_jobs.sql:283-291 (ListUserJobs) — `CASE WHEN a.applied_at IS NOT NULL THEN GREATEST(a.applied_at, (SELECT max(e.received_at) …))` correlated on `(e.user_id, e.job_id)`.
- internal/db/queries/mail_linking.sql:7-14 (GetUserApplication) — same expression again; the header comment at mail_linking.sql:3-5 says it "mirror[s] ListUserJobs deliberately… two derivations of one rule drift".
- internal/db/queries/ghost.sql:25-30 — third copy; ghost.sql:18-19 repeats the same "one definition of 'when did this application last move', not two" claim.
- internal/db/queries/applications.sql:62-68 (ListOrphanedApplications) — fourth copy, correlated through `e.application_id = a.id` instead of `(user_id, job_id)`.
- internal/db/queries/user_jobs.sql:300 — `has_pending_suggestion` tests `AND e.job_id IS NULL`.
- internal/db/queries/mail_linking.sql:23 and internal/db/queries/ghost.sql:37 — the same predicate tests `AND e.application_id IS NULL`.
- internal/db/queries/mail_classification.sql:46-49 — `application_id` is explicitly "Left NULL when the user has no application for that posting", so `application_id IS NULL` is strictly broader than `job_id IS NULL`.

**Verifier.** The SQL citations are accurate (user_jobs.sql:284-291 and the predicate at :301 — cited as :300, off by one; mail_linking.sql:7-14,23; ghost.sql:25-30 and :38 — cited as :37; applications.sql:62-68), and the last-activity expression really is written three times (the fourth, ListOrphanedApplications, MUST differ — job_id is NULL for those rows, so it correlates through application_id; a shared (user_id, job_id) view cannot serve it, so the proposed remedy is partly infeasible as stated). The claimed live disagreement is refuted: for the board and the gate to differ, a row must have `suggested_job_id` set together with either a non-NULL job_id + NULL application_id, or a NULL job_id + non-NULL application_id. Neither is reachable. suggested_job_id is written only by SetEmailClassification (mail_classification.sql:50), whose caller maillink/decide.go:34-42 returns job_id and suggested_job_id mutually exclusively; every other path that touches it (ConfirmEmailLink :46, RejectEmailLink :52, LinkEmailToJob :63, AgentTriageEmail mail_classification.sql:95) sets it to NULL. And application_id is always derived from job_id, so it is NULL whenever job_id is. The two predicates evaluate identically on every row the system can produce — a latent inconsistency in wording, not the badge-vs-gate contradiction described.

#### ✅ 28. The "one channel abstraction" the docs promise is two forked copies — Notifier, Router, ErrChannelNotConfigured and recipient() all exist twice, and have already drifted

`notification-channel-seam-forked-in-two` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** notifications.md:14-17 states "A new channel is a new `Notifier` implementation, never an engine change… Adding webhooks means adding a package, not touching `notify` or `reminder`." That is false today. Adding a webhook channel means: a new type in internal/reminder/transports.go (touching `reminder`), a new package or type for the digest side, a new case in BOTH `recipient` functions, and a new `if` in BOTH cmd mains. Five edit sites for one channel. The drift is not hypothetical — the two `recipient` functions already disagree about what an unregistered channel resolves to, so a channel that stores its destination in the `destination` column works for subscriptions and is silently undeliverable for reminders.

**Remedy.** Do not introduce notify.Router[T]/Notifier[T] generics or an exported notify.Recipient taking pgtype args (that would make notify depend on both delivery row shapes — worse coupling than the duplication). Two cheap, proportionate moves: (1) correct notifications.md:9 — emailnotify/telegramnotify implement notify.Notifier only, the reminder transports live in internal/reminder; (2) if the duplicate allowlist maps bother you, export `notify.ValidChannel(string) bool` beside `notify.Channels` and delete the two hand-rolled maps at subscription.go:43 and reminder.go:46. Revisit collapsing the Router/Notifier pair only when a third channel actually lands.

**Evidence.**

- docs/agents/notifications.md:9 — table row claims `internal/emailnotify` "implements both `Notifier` interfaces"; it implements only `notify.Notifier` (internal/emailnotify/notifier.go:19). The reminder-side email transport lives in internal/reminder/transports.go:55.
- internal/notify/notify.go:60 `type Notifier interface` vs internal/reminder/engine.go:25 `type Notifier interface` — same signature, different payload struct.
- internal/notify/router.go:17,23 `ErrChannelNotConfigured` + `type Router map[string]Notifier` vs internal/reminder/engine.go:32,36 — byte-for-byte parallel, including the soft-skip contract.
- internal/notify/notify.go:154 `recipient(info db.GetSubscriptionForDeliveryRow)` vs internal/reminder/engine.go:203 `recipient(channel string, info db.GetReminderForDeliveryRow)` — same telegram-chat-id/account-email rule, ALREADY DRIFTED: notify falls through to the stored `Destination` column for an unknown channel, reminder returns `("", false)`.
- internal/reminder/transports.go:23 `TelegramNotifier` and :55 `EmailNotifier` render reminders inside `reminder`, while the digest renderers for the same two channels live in internal/telegramnotify/notifier.go and internal/emailnotify/notifier.go.
- cmd/notify/main.go:50-63 vs cmd/remind/main.go:43-56 — identical "build the router from config, disable a channel on client error, exit 0 if empty" blocks, differing only in the Router type and constructor names.
- internal/subscription/subscription.go:43 and internal/reminder/reminder.go:46 — the same `map[string]bool` is rebuilt from `notify.Channels` in both, because notify exposes the slice but no membership test.

**Verifier.** Every citation is real: notify/notify.go:60 vs reminder/engine.go:25 (same-signature Notifier), notify/router.go:17,23 vs reminder/engine.go:32,36 (byte-for-byte parallel Router + ErrChannelNotConfigured), transports.go:23,55, cmd/notify/main.go:50-63 vs cmd/remind/main.go:43-56, subscription.go:43 vs reminder.go:46 (two identical 6-line validChannels maps). So the parallelism is real. But the load-bearing claim — 'ALREADY DRIFTED... a channel that stores its destination in the destination column works for subscriptions and is silently undeliverable for reminders' — is false. `destination` is a subscriptions column only (migrations/0001_init.sql:341); job_reminders stores `channels text[]` and NO destination (migrations/0034_saved_job_reminders.sql:24,39; reminders.sql:81-93 GetReminderForDelivery selects no destination). reminder.recipient (engine.go:203-217) therefore CANNOT fall through to a stored destination — there is none to fall through to. The two functions resolve over two different row shapes, not one decision written twice; the only truly shared logic is ~10 lines of telegram-chat-id/account-email resolution. Nothing is broken today, and the third channel that would make the seam hurt does not exist — building for it is exactly the 'infrastructure before there's a concrete need' the project forbids. The one thing the code genuinely contradicts is a doc line: notifications.md:9 says internal/emailnotify 'implements both Notifier interfaces' when emailnotify/notifier.go:19 asserts only `notify.Notifier` and the reminder-side email transport lives in reminder/transports.go:55.

#### ✅ 29. Only the pure stage→threshold mapping is shared; the actual silence derivation (day math + last-activity fallback + "is this even an application") is written three times, and the follow-up gate's copy is missing a guard the board's copy deliberately keeps

`silence-verdict-derived-three-times` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** Three callers each rebuild the same verdict from raw columns and only share the last, cheapest step. The invariant "the badge and the follow-up offer agree" now rests on two SQL statements producing identical `last_activity_at`, plus three Go functions independently flooring days and independently deciding what counts as an application. The board's copy defends itself against a NULL `last_activity_at` by falling back to `applied_at`; the follow-up handler does not, so any path that yields a NULL there (a query change, an application row read through a join that misses the GREATEST) makes the board show an amber "24d" badge with a follow-up button that answers 409. Three copies of "never negative, part-days do not count" are held together by comments naming each other.

**Remedy.** Export the day math once — `func DaysSilent(now, last time.Time) int` in internal/userjob beside SilenceStateFor — and have the three sites call it. Do not move the whole derivation into a single userjob.Silence(...): the applied_at fallback and the 'is this even an application' precondition are shaped by two different query results (jobtracking's TrackedJob pointers vs the flat GetUserApplicationRow vs ghost's already-filtered rows), and ghost deliberately does NOT want jobtracking's applied_at fallback semantics (ghost.sql:10-16 explains why for the public claim).

**Evidence.**

- internal/userjob/silence.go:72 `SilenceStateFor(stage, daysSilent, pendingSuggestion)` — the shared part: a pure mapping that takes an already-computed day count.
- internal/jobtracking/silence.go:32-56 — derivation #1: returns nil when `AppliedAt == nil`, falls back `last = max(AppliedAt, LastActivityAt)`, `days := int(now.Sub(last).Hours()/24)` floored at 0. Its own comment (silence.go:29-31) says the applied_at fallback is repeated in Go on purpose, "so the domain [does not reintroduce] a hole the SQL closed".
- internal/handler/followup.go:46-52 — derivation #2: uses `app.LastActivityAt` alone with no applied_at fallback and no `AppliedAt != nil` precondition, then calls `SilenceStateFor` directly.
- internal/handler/followup.go:94-99 `daysSilent(now, last)` — a second copy of the same floored day math, in a different shape from jobtracking's.
- internal/ghost/evidence.go:73 and :107-114 `daysSince` — derivation #3, whose comment says it exists "matching jobtracking.Silence so the two surfaces cannot disagree by a day".
- internal/userjob/AGENTS.md:27 — claims `GET /me/tracking/:slug/followup` refuses anything not `silent` "reusing `SilenceStateFor` so the offer and the badge can never disagree".

**Verifier.** Line numbers check out (userjob/silence.go:72, jobtracking/silence.go:32-56, handler/followup.go:46-52 and 94-99, ghost/evidence.go:73,107-113, userjob/AGENTS.md:27). But the stated failure — 'board shows an amber 24d badge with a follow-up button that answers 409' because followup.go omits the applied_at fallback — is not reachable. Both queries wrap the expression in `CASE WHEN a.applied_at IS NOT NULL THEN GREATEST(a.applied_at, max(e.received_at))` (mail_linking.sql:7-14, user_jobs.sql:284-291), and Postgres GREATEST ignores NULL arguments, so last_activity_at is NULL if and only if applied_at is NULL — i.e. the row is not an application. In that case followup.go computes days=0, SilenceStateFor returns `active`, and the handler 409s, which is the correct answer; jobtracking.Silence returns nil for the same row, so board and gate agree. The applied_at fallback in jobtracking is explicitly documented (silence.go:29-31) as defensive repetition of a guarantee the SQL already gives, not a hole the handler forgot. What actually survives is much smaller: three copies of a 4-line 'whole days, floored at zero' helper (jobtracking/silence.go:48-51, handler/followup.go:94-99, ghost/evidence.go:107-113). The ladder itself — the part that could actually disagree — is already shared through SilenceStateFor at all three sites, exactly as userjob/AGENTS.md:27 promises.

### Shared infrastructure and the 43 binaries

> **Reviewer's note on what is well-factored here.** The shared bootstrap is a genuine, successful answer, not a fig leaf. 33 of the 34 production cron binaries call worker.Bootstrap + worker.Main, and per-main boilerplate really is down to ~8 lines (cmd/recount-companies/main.go:20-38 is the whole binary at 39 lines). worker.ExitCode / worker.Heartbeat are small, documented, and used where they matter. The remaining non-Bootstrap mains — harvest-*, gen-cities, gen-contracts, cv-previews — are explicitly exempted as dev tools by internal/observability/AGENTS.md, and cmd/server's own signal/shutdown story is legitimately different.

Configuration IS one typed struct plus five scoped loaders (Settings/Enrich/Embed/EmbedClient/Reindex/Credits), not scattered os.Getenv: outside internal/config there are only ~20 os.Getenv sites in ~101k LOC, and nearly every one carries a written reason (internal/sources/registry.go:269-282 — keys are secrets that must gate registration; internal/resume/resume.go:32-36 — why PDFTOTEXT_BIN is not threaded through config). internal/database's pool_max_conns detection (database.go:45-62) and internal/migrate are both tight and self-contained. pgconv/pgerr are correctly small, and the 24 pgtype.Timestamptz literals outside pgconv are all non-optional values, which pgconv deliberately does not cover — not a bypass.

Deliberately NOT flagged: the 5-line envInt/envOr helpers redeclared in cmd/backfill-semantic-vectors:108, cmd/rollup-views:220 and cmd/backfill-derive:132 (too small to justify a seam; config's are unexported on purpose); the keyset-page loop repeated across the backfills as a general shape (a generic page iterator would be premature generics — the specific ResilientPage bypass above is the real defect); cmd/server not using worker.Bootstrap; the repeated `if cfg.MeiliKey == "" { return 1 }` guard in six workers (two lines each, worker-specific message); cmd/cv-previews:21-27 reimplementing config.resolveTypstBin (a dev tool that genuinely wants fatal, not "disabled"); and cmd/embed:46 reading EMBED_PG_ONLY via os.Getenv rather than config.LoadEmbed (one knob, arguably a mode switch rather than tuning).

#### 30. Two production LLM-spending backfill workers bypass worker.Bootstrap/worker.Main, losing Sentry, SIGTERM handling and the exit-code convention

`backfill-workers-off-the-shared-bootstrap` · reuse · severity **medium** · effort **S** · verdict **confirmed**

**Problem.** These are the only two DB-writing, prod-data workers outside the shared path (verified: every other non-Bootstrap cmd is a documented dev tool or cmd/server). Both spend LLM money on stored user CVs. Because neither calls observability.Init, a panic or run-ending error in either is invisible to Sentry — the exact gap worker.Main was written to close. Because neither derives a signal context, a redeploy's SIGTERM kills them mid-extraction instead of unwinding. And backfill-experience's os.Exit(1) at :136 silently drops the buffered Langfuse traces that would explain why the run failed.

**Remedy.** As proposed, and it is cheap. One addition worth naming: log.Fatalf appears six more times in backfill-resume-structured (:52, :64, :67, :79, :83, :89, :108, :120, :126) — each also skips the deferred flush, so converting to `return 1` should cover those, not just the two the finding names.

**Evidence.**

- cmd/backfill-experience/main.go:62 — `cfg := config.Load()` then `ctx := context.Background()` and `database.Connect` at :65, hand-rolling exactly what worker.Bootstrap does
- cmd/backfill-experience/main.go:136 — `os.Exit(1)` fires with `defer pool.Close()` (:69) and `defer flush()` (:76) pending; os.Exit runs no deferred function, so the Langfuse trace flush is skipped precisely on the partially-failed run
- cmd/backfill-resume-structured/main.go:47-54 — same hand-rolled config.Load + context.Background + database.Connect
- cmd/backfill-resume-structured/main.go:161 — counts `failed` and then returns normally; the process always exits 0, so cron cannot alert on a run that failed every user
- internal/worker/bootstrap.go:29-53 — Bootstrap is the shared path: observability.Init, signal.NotifyContext(SIGINT/SIGTERM), pool, one cleanup
- internal/observability/AGENTS.md — "`observability.Init` lives in `worker.Bootstrap`" and "Every cron worker's `main` uses `worker.Main(run)`"; only `harvest-*`/`gen-contracts` are declared out of scope

**Verifier.** Every citation checks out. I enumerated all 43 cmd/ dirs for worker.Bootstrap/worker.Main: the only non-users are server, gen-cities, gen-contracts, harvest-* (declared out of scope in internal/observability/AGENTS.md), cv-previews (no DB at all — cmd/cv-previews/main.go:4 "no database"), and exactly the two named. cmd/backfill-experience/main.go:62-65 hand-rolls config.Load + context.Background + database.Connect; :136 `os.Exit(1)` runs with `defer pool.Close()` (:69) and `defer flush()` (:76) pending, and flush() at :76 is the Langfuse tracer drain returned by llm.NewClient at :205-212 — so the LLM traces really are dropped precisely on the failed run. cmd/backfill-resume-structured/main.go:47-50 is the same hand-roll, and :161-162 logs `failed` then falls off the end of main, so the process exits 0 even when every user failed; it also uses log.Fatalf at :52/:64/:79/:108/:120/:126, each of which skips the `defer llmFlush()` at :81. This contradicts internal/observability/AGENTS.md, which scopes the exemption to harvest-*/gen-contracts only. Downgrading from high: both are operator-run one-shot seeds (not cron-timed units — nothing in the repo schedules them), so the missing SIGTERM context is largely theoretical, and an operator watching the terminal sees the log lines Sentry would have carried. The exit-code and flush defects are real but narrow.

#### 31. worker.ResilientPage is the shared corruption-tolerant full scan over jobs, but only cmd/reindex uses it — two backfills run the identical ListJobsByIDAfter keyset scan raw

`resilient-full-scan-bypassed-by-backfills` · reuse · severity **medium** · effort **M** · verdict **confirmed**

**Problem.** ResilientPage exists because one damaged TOAST pointer fails an entire `SELECT *` page (internal/worker/resilient.go:16-19). cmd/backfill-derive is described in CLAUDE.md as the worker that "re-derives every deterministic column (facets, role_fingerprint, slugs) in one keyset pass" — a whole-catalogue pass is its entire purpose, and a single XX001 row aborts it with nothing derived past that id. cmd/backfill-descriptions has the same exposure. The shared answer was written, tested, and then applied to exactly one of the three full-table scanners.

**Remedy.** Wire cmd/backfill-derive only: widen its store interface to the three methods worker.jobQueries names, build worker.NewFullScanReader(q), and replace the loop at :244-256 with ResilientPage using the `lastID == afterID` exhaustion test. Do NOT add a third PageReader constructor for backfill-descriptions' source-scoped path — that is a one-off historical encoding repair, and inventing a constructor to serve it is exactly the infrastructure-ahead-of-need the project forbids; if it matters at all, its unscoped branch can use NewFullScanReader and its scoped branch can stay as-is.

**Evidence.**

- internal/worker/resilient.go:90-141 — ResilientPage: on SQLSTATE XX001 it re-lists the window as bare ids and fetches rows one by one, skipping the damaged row so the scan continues
- internal/worker/resilient.go:52-64 — NewFullScanReader wraps exactly `ListJobsByIDAfter` / `ListJobIDsAfter` / `GetJob`
- cmd/reindex/main.go:153 and :338 — the only consumer
- cmd/backfill-derive/main.go:245-247 — `store.ListJobsByIDAfter(ctx, db.ListJobsByIDAfterParams{AfterID: afterID, BatchSize: backfillBatchSize})` inside the producer goroutine; any error calls `fail(e)` at :251, which cancels the whole run
- cmd/backfill-descriptions/main.go:188-195 — pageJobs issues the same `ListJobsByIDAfter`; the error check at cmd/backfill-descriptions/main.go:119-121 aborts backfillAll outright

**Verifier.** Citations verified exactly: internal/worker/resilient.go:52-64 (NewFullScanReader over ListJobsByIDAfter/ListJobIDsAfter/GetJob), :101-141 (the XX001 degrade path), and grep confirms cmd/reindex/main.go:153, :156, :338 are the only non-test consumers. cmd/backfill-derive/main.go:245-247 issues the raw ListJobsByIDAfter inside the producer goroutine and cmd/backfill-descriptions/main.go:188-194 does the same via pageJobs, aborting at :119-121. What makes this more than tidiness: the commit that added ResilientPage (1153d215, "feat(workers): survive corrupted (XX001) rows in full-scan workers") says a broken TOAST pointer had already crashed a full facet reindex on this prod database — so XX001 is an observed condition here, not hypothetical. And cmd/backfill-derive has NO resume flag (only BACKFILL_CONCURRENCY at :133), so a single corrupted row makes the whole-catalogue re-derive permanently unable to finish past that id, re-failing at the same place every run. That is a blocked operational worker, which is what medium means.

#### ✅ 32. Postgres session advisory-lock single-flight is hand-rolled three times with no shared key namespace, and internal/migrate still asserts it is the only user

`advisory-lock-single-flight-triplicated` · coupling · severity **low** · effort **S** · verdict **overstated**

**Problem.** The same ~16-line ritual (acquire a dedicated conn, pg_try_advisory_lock, exit 0 if not held, unlock + release on the way out) appears in three places and has already drifted into two different release shapes between liveness and ghost-crosscheck. There is no single home for the lock keys, so the fourth worker needing single-flight has nowhere to look to avoid a collision — and the one comment that would have told it to look is the one asserting no other users exist. The stale CLAUDE.md flock claim shows the gap is already making operators reason about the wrong mechanism.

**Remedy.** Fix the stale sentence at internal/migrate/migrate.go:39-40 (say the other users are the cmd/liveness and cmd/ghost-crosscheck try-locks and name their keys). Do NOT move migrate's key into internal/worker — worker/bootstrap.go pulls config, database and observability, and making the migration runner import all three to host one int64 const is strictly worse than the duplication. A tiny `worker.TryAdvisoryLock(ctx, pool, key) (release func(), held bool, err error)` for the two try-lock sites is defensible but optional at two callers.

**Evidence.**

- internal/migrate/migrate.go:38-41 — "it only needs to not collide with other advisory-lock users, and the project has none" — factually stale
- internal/migrate/migrate.go:162-170 — pg_advisory_lock(728391) + deferred unlock on a dedicated conn
- cmd/liveness/main.go:45 — `lockKey = 0x66686c76`; :73-93 — Acquire, pg_try_advisory_lock, three separate manual `lockConn.Release()` calls on the error / not-locked / success paths
- cmd/ghost-crosscheck/main.go:45 — `lockKey = 0x66686763`; :66-81 — the same 16 lines in a different shape: one `defer lockConn.Release()` at :70 covering all paths, with the unlock defer registered after it
- CLAUDE.md — "`reindex-companies` and `rollup-views` hold their own flock", but grep finds no flock anywhere in cmd/ or internal/ — the operational doc describes a mechanism the code does not have

**Verifier.** Line numbers are right (internal/migrate/migrate.go:38-41 and :162-171; cmd/liveness/main.go:45 and :73-93; cmd/ghost-crosscheck/main.go:45 and :66-81), but three of the four supports fail. (1) It is duplicated twice, not three times: migrate uses blocking pg_advisory_lock, a deliberately different decision the finding itself concedes, so it is not an instance of the try-lock ritual. (2) The "drift into two different release shapes" is cosmetic — both are correct. ghost-crosscheck's `defer lockConn.Release()` at :71 followed by the unlock defer at :81 unwinds LIFO (unlock, then release), and liveness's three manual Release() calls cover the same paths. Neither leaks. (3) The flock claim is simply wrong. flock is the cron/systemd unit wrapper, not Go code, and the repo says so in three places: cmd/reindex-companies/main.go:4 ("on its own flock"), internal/llm/llm.go:27 ("holding its cron flock open"), internal/sources/yandex.go:82 ("it hung a prod custom.yml ingest for ~4h, holding its flock"). CLAUDE.md:52 describes ops reality accurately; the finding's "grep finds no flock anywhere" is a false negative. The keys are also ASCII-derived and self-documenting (0x66686c76 "fhlv", 0x66686763 "fhgc"), each commented "unique to this worker" — collision is not a live hazard. What survives is small: two ~16-line copies, and migrate.go:40's "the project has none" is now factually stale.

#### 33. SQLSTATE classification is split between internal/pgerr and internal/worker, contradicting pgerr's own "single home" claim

`sqlstate-classification-split-from-pgerr` · boundary · severity **low** · effort **S** · verdict **confirmed**

**Problem.** internal/worker now carries a Postgres SQLSTATE constant and its own unwrap, so a package whose stated job is "shared bootstrap and run-outcome plumbing" (internal/worker/exit.go:1-5) also owns a piece of the persistence-error taxonomy. The next code that needs to recognise XX001 — a repository, or the handler's error renderer — will either import internal/worker (pulling in the pool, config and observability with it) or write a third copy of the same three lines. Small, but it is the seam sitting in the wrong package rather than a style preference.

**Remedy.** Move corruptDataSQLState and IsCorruptedRow into internal/pgerr as `pgerr.IsDataCorrupted`, next to IsUniqueViolation/IsForeignKeyViolation. internal/worker/resilient.go calls it at :109 and :125; the resilient-scan policy (which errors opt into the degrade path) stays in worker, only the SQLSTATE recognition moves.

**Evidence.**

- internal/pgerr/pgerr.go:1-5 — "It is the single home for the SQLSTATE constants the repositories and the central error handler share"
- internal/pgerr/pgerr.go:14-17 and :21-24 — codeUniqueViolation/codeForeignKeyViolation plus the errors.As(*pgconn.PgError) unwrap
- internal/worker/resilient.go:19 — `const corruptDataSQLState = "XX001"` declared in a second package
- internal/worker/resilient.go:24-27 — `IsCorruptedRow` repeats the identical errors.As(&pgErr) && pgErr.Code == … body

**Verifier.** All four citations are exact: internal/pgerr/pgerr.go:1-5 claims to be "the single home for the SQLSTATE constants", :14-17 and :21-30 hold the codes and the errors.As(*pgconn.PgError) unwrap, and internal/worker/resilient.go:19 and :24-27 are a second declaration of the same pattern in a package whose own doc (worker/exit.go:1-5) scopes it to "shared bootstrap and run-outcome plumbing". The finding's predicted consequence has already happened, which strengthens it: internal/enrich/runner.go:123 and internal/embed/runner.go:182 import internal/worker for nothing but IsCorruptedRow (grep confirms those are their only worker.* references), so two domain packages now transitively depend on config, database and observability via worker/bootstrap.go to classify one SQLSTATE. The remedy is a three-line move with no new abstraction, and the policy (which errors opt into the degrade path) correctly stays in worker. Low is the right severity — nothing misbehaves today.

#### ✅ 34. The same six LLM/Langfuse env fields are declared in three struct types and hand-copied at seven construction sites, and the copies have already drifted

`llm-settings-restated-three-times` · reuse · severity **low** · effort **M** · verdict **overstated**

**Problem.** internal/llm/llm.go:138-144 states NewClient is "the single construction path" so "no entrypoint can build a client and forget to wire tracing" — but the settings feeding it are assembled by hand seven times. Adding a seventh field to llm.Settings (a request timeout, an org header, a fallback model) means editing seven call sites plus deciding which of the two config structs grows it; missing one silently disables that feature for one worker, exactly as the assistant's cmp.Or fallback exists in only one of the seven. config.Enrich additionally re-reads env vars config.Load already read, so cmd/enrich (which calls both, at main.go:25 and via Bootstrap at :48) parses LLM_* twice into two shapes.

**Remedy.** Kill the actual redundancy first, without a new dependency edge: read the six env vars once in internal/config (one unexported loader) and have both Settings and Enrich carry that one field group, so LoadEnrich stops re-parsing what config.Load already read and cmd/enrich stops parsing LLM_* twice. Keep the llm.Settings mapping at the cmd boundary exactly as EmbedClient does — or, if you want one mapping, put it on the config side only after config already depends on nothing else in internal/, and keep cmd/server's assistant model as an explicit field assignment on the result so the override stays visible.

**Evidence.**

- internal/config/config.go:192-205 — Settings reads LLM_BASE_URL/LLM_API_KEY/LLM_MODEL/LANGFUSE_*
- internal/config/enrich.go:42-51 — LoadEnrich reads the SAME six env vars into a second struct
- internal/llm/llm.go:121-131 — llm.Settings is a third struct with the same six fields
- cmd/enrich/main.go:34-41, cmd/tg-extract/main.go:46-53, cmd/classify-mail/main.go:32-39 — three identical six-line field-by-field copies from config.Enrich into llm.Settings
- cmd/backfill-experience/main.go:205-212 and cmd/backfill-resume-structured/main.go:70-77 — two more, from config.Settings
- cmd/server/main.go:143-150 vs cmd/server/main.go:160-167 — the drift: the second copy substitutes `cmp.Or(cfg.AssistantModel, cfg.LLMModel)` for Model while the other six sites pass the model straight through
- internal/config/embed.go:47-51 — the established precedent: EmbedClient lives in config "rather than inside internal/search so the library stays env-free"

**Verifier.** The duplication is real and the counts are right: internal/config/config.go:192-205 and internal/config/enrich.go:42-51 both read LLM_BASE_URL/LLM_API_KEY/LLM_MODEL/LANGFUSE_*, internal/llm/llm.go:121-131 is the third struct, and grep finds exactly seven `llm.Settings{` construction sites (backfill-resume-structured:70, classify-mail:32, server:143, server:160, backfill-experience:205, tg-extract:46, enrich:34). The double-parse in cmd/enrich is real too (LoadEnrich at :25, then config.Load inside worker.Bootstrap at :48), and config.go:19-21's stated reason for the split — "Only the server calls Load — the enrich worker reads its own LoadEnrich" — is now stale. But the headline claim is false: cmd/server/main.go:160-167 has not "drifted". The cmp.Or is a deliberate, commented decision at :156-159 ("The in-app assistant runs on its own model: ASSISTANT_MODEL is chosen for reliable tool calling... Unset falls back to LLM_MODEL"). Citing an intentional per-client override as evidence of accidental divergence is the finding's main support, and it does not hold. The cited precedent also cuts the other way: internal/config/embed.go:47-56 shows config holding a PLAIN struct while "cmd wires them into search.NewClient's WithEmbed* options" — config imports nothing from internal/ today (config.go:3-10 imports only stdlib). So the precedent argues against `func (s Settings) LLM() llm.Settings`, not for it. Real risk reduces to: adding a seventh field to llm.Settings compiles fine and silently zeroes it at seven sites. Annoying, not medium.

#### 35. The catalogue-wide dedup campaign — three ordered passes plus the fuzzy-similarity thresholds — lives in package main under cmd/reindex, while the write-path half it must agree with lives in internal/jobdedup

`batch-dedup-trapped-in-package-main` · boundary · severity **low** · effort **L** · verdict **overstated**

**Problem.** Two tiers of one decision must stay in step (jobdedup's synchronous canon vs the batch's MIN(id) canon), and the contract between them is a doc comment because one tier is in package main. Nothing outside cmd/reindex can invoke, test against, or reuse the collapse: cmd/backfill-derive, which re-derives every role_fingerprint, can only tell the operator to run a different binary afterwards. Changing the canon rule means editing two packages that cannot reference each other, with no compiler or test able to catch a divergence.

**Remedy.** No move is warranted today: internal/jobdedup would gain an exported CollapseAll with exactly one caller. The residue worth fixing is smaller — the fuzzy clustering (fuzzy.go:139-163, plus the two thresholds) is a pure function over jobhash signatures with no db dependency; if a second caller ever appears, that is the piece to lift, into internal/jobhash next to DescriptionSignature/DescriptionSimilarity. Until then leave it.

**Evidence.**

- cmd/reindex/fuzzy.go:15 — `fuzzyThreshold = 0.9`, and :23 `fuzzyMaxBucket = 200`: the catalogue's definition of "the same posting", declared in package main
- cmd/reindex/fuzzy.go:42-88 and :100-140 — collapseFuzzyDuplicates / bucketByRole / clusterBucket, the whole clustering rule, unreachable from any other package
- cmd/reindex/main.go:118-148 — three passes whose ORDER is load-bearing ("Run AFTER the role recompute", "Runs LAST so it only ever claims what they did not"), enforced only by comments inside one main()
- cmd/reindex/main.go:401-437 — recomputeRoleDuplicates / suppressAggregatorDuplicates, also package main
- internal/jobdedup/jobdedup.go:1-13 — the package exists for exactly this question and documents that its synchronous answer "is deliberately the one the batch would give" — but it cannot see or call the batch
- cmd/backfill-derive/main.go:42-44 — "Follow the run with a reindex (make reindex), whose duplicate_of recompute then collapses any newly-clustered reposts": a second worker depends on the pass and can only reach it by shelling out

**Verifier.** The citations are roughly right (fuzzyThreshold is at cmd/reindex/fuzzy.go:16, not :15; collapseFuzzyDuplicates is :42-50 with the per-company body at :52-94; bucketByRole :111-125, clusterBucket :139-163), but the framing is wrong on three counts. (1) The canon rule is NOT in package main. cmd/reindex/main.go:406-412 and :419-436 are ten-line wrappers over sqlc calls; the actual MIN(id) rule lives in internal/db/queries/jobs.sql (RecomputeRoleDuplicatesForCompany at :423, CanonicalJobForRole at :316, whose comment explicitly says it "Mirrors the canon RecomputeRoleDuplicatesForCompany picks (MIN(id) among the cluster's open rows)"). Changing the canon means editing two queries in one file, not two Go packages. (2) The two-tier contract is not "a doc comment" — internal/pipeline/AGENTS.md documents it at length ("Role-cluster dedup is two-tier, and the tiers must agree on the canon or they undo each other... The synchronous answer deliberately refuses a canon NEWER than the row being written"). This is a documented, reasoned boundary, not an accident. (3) "Nothing can test against it" is false: cmd/reindex/fuzzy_test.go and main_test.go exist and exercise clusterBucket/bucketByRole through the fuzzyDedupQuerier interface declared at fuzzy.go:28-33 for exactly that purpose. The backfill-derive point is also weak: cmd/backfill-derive/main.go:42-44 tells the operator to run `make reindex` because moving role_fingerprints invalidates the Meili index, which requires a reindex regardless of where the collapse code lives.

### SvelteKit app and design system

> **Reviewer's note on what is well-factored here.** Genuinely well-factored, and I deliberately did NOT flag it:

- **The filter/facet subsystem is one coherent model, not several.** The prompt's hypothesis does not hold up. `facetModel.ts` (pure job model) and `companyFacetModel.ts` (pure company model) are each wrapped by exactly two reactive surfaces — a URL-synced store (`filters.ts` / `companyFilters.ts`) and a staged modal copy (`stagedFilters.svelte.ts` / `stagedCompanyFilters.svelte.ts`) — and all four implement the one `FacetStore` interface declared at `web/src/lib/facets.ts:62`, which is why `FilterModalShell.svelte` + `FacetSection.svelte` render both catalogues unchanged. `filterControls.ts` (slider bounds) and `filterSections.ts` (rail grouping) are presentation registries over that model, not competing models. `filterSections.ts:36` keys `CATEGORY_GROUP` by the full generated `Category` union so a new backend category is a compile error — the type system enforcing the contract, exactly what the boundary lens asks for. There is ~40 lines of mutator-forwarding duplicated between `FilterStore` and `StagedFilters`, but the two differ precisely in commit policy (`setNow`/`setSoon` vs in-memory), which is the whole point of the staged surface; collapsing them would cost more indirection than it removes.

- **`types.ts` vs the generated `contracts.ts` is not a drift hazard.** It re-exports rather than re-declares wherever generation exists (`types.ts:12-24`, `:33-41`, `:734-742`) and even uses inline `import('./generated/contracts').Structured` at `:751`. The hand-written types are the ones `cmd/gen-contracts/main.go:81-170` deliberately does not cover (handler-assembled responses: Company, User, MyJob, referrals, submissions). No hand-written type shadows a generated one. The real seam problem is *where* the non-generated ones live, which is finding `mail-wire-shapes-split-across-api-and-types`.

- **`api.ts` at 1729 lines is one client module, not a dumping ground.** Every fetch in `web/src` outside `lib/assistant/` and `lib/server/og/logo.ts` goes through it; `serverApi` (`lib/server/api.ts:16`) is the single SSR binding and every `+page.server.ts` uses it. I tested the obvious failure — that the 161-name `return {}` block at api.ts:1480-1723 is a hand-maintained second copy of the surface that has drifted — and it has not: all 8 functions missing from it are the internal helpers (`call`, `request`, `requestData`, `jsonBody`, `insightsQuery`, `postAuth`, `jobInteraction`, `resumeInit`). Splitting it by domain is defensible but I would be trading a long file for a new import graph with no defect to point at.

- **`UserResource` (`lib/userResource.svelte.ts`) is the right amount of machinery**: a self-registering registry so the sign-out sweep cannot forget a store, with a generation guard against same-tab user handoff. State sharing via `.svelte.ts` singletons is coherent — I found no duplicated `$effect` reload logic across components, only repeated `ensureLoaded()` calls, which is the idempotent design working as intended.

- **Not flagged: low design-system adoption per se** (11 of 15 primitives at zero). `design-system/AGENTS.md` names it explicitly and `check:adoption` reports it every run, so it is a tracked state, not a discovery. I flagged only the case where an unused primitive has *six* app-side twins. `Avatar` (baseline 0) has a parallel implementation in `lib/avatar.ts` and `EmptyState` (baseline 0) overlaps `States.svelte`'s empty branch, but both app versions are deliberately different (a curated dark palette vs the DS pastel hue; a tri-state loading/empty/error vs a rich empty state) and neither has drifted — adjacent, not defects.

- **Not flagged: `HomeView`/`JobsView` near-duplication.** They share no markup; `HomeView.svelte` is a marketing landing built from static illustrative arrays (lines 30-100) and `JobsView.svelte` is the live feed. `JobRow.svelte` is genuinely the single card for every list surface (feed, company page, saved, board, assistant deck) via its `compact`/`newTab`/`dimViewed`/`footer`/`onHide` props. `SwipeDeck`'s `.job-teaser` styles are a deliberate compact variant of the description CSS, not a copy — unlike JobDrawer's, which is byte-identical.

- **Borderline, not raised as its own finding:** `lib/assistant/api.ts:27` re-implements `credentials`+envelope-unwrap (`lib/api.ts:266-290`) and throws a bare `Error` where `api.ts` throws a status-carrying `ApiError`. It is ~15 lines and the assistant is a self-contained sub-app with its own SSE transport; I judged it deliberate rather than accidental.

#### ✅ 36. The "single source of display labels" is forked: two exported categoryLabel functions, four humanize fallbacks, and a relocation map that has already drifted

`category-label-fallback-forked` · reuse · severity **medium** · effort **S** · verdict **confirmed**

**Problem.** `CATEGORY_VALUES` carries 36 codes and `CATEGORY_LABELS` overrides only 10, so 26 of them render through a fallback — and there are two fallbacks that disagree on casing. A user filtering by "Network Engineering" opens a job whose Category row says "Network engineering", and the SEO insights pages use a third copy of the rule. Relocation is worse: the same enum value is labelled "None" in the filter and "Not supported" on the detail page. The file that exists to prevent precisely this drift is bypassed by the two vocabularies most likely to grow.

**Remedy.** Keep the remedy, minus one item: headerScope.ts:24's titleCase only ever runs over WORK_MODE codes (remote/hybrid — single words, 'onsite' already mapped), so it can never diverge and folding it in is churn. Do the three that matter: one exported humanize/labelFor pair in labels.ts consumed by enrichment.ts:47 and facets.ts:305; a RELOCATION_LABELS in labels.ts read by both enrichment.ts:191 and facets.ts:358 (pick one wording); and collapse insights.ts:15/78 onto labels.ts's CATEGORY_LABELS + the single categoryLabel — note insights.ts's map carries values labels.ts lacks (fullstack:'Full-Stack'), so the merge must move those overrides into labels.ts rather than drop them.

**Evidence.**

- web/src/lib/labels.ts:1 — "Single source of display labels for the closed-vocabulary facet codes, shared by the detail-page facet rows (enrichment.ts) and the filter panel (facets.ts) … Keeping ONE map prevents the drift that previously left stale region codes and inconsistent casing in two places"
- web/src/lib/enrichment.ts:31 — a private `RELOCATION` map: `not_supported: 'Not supported'` — bypasses labels.ts entirely
- web/src/lib/facets.ts:358 — the same vocabulary again with a different label: `not_supported: 'None'`; so the job detail page's Relocation row and the filter pill for the same code read differently
- web/src/lib/enrichment.ts:47 — `humanize()` uppercases only the first character: `network_engineering` → "Network engineering"
- web/src/lib/facets.ts:305 — a second `humanize()` title-cases every word: `network_engineering` → "Network Engineering"
- web/src/lib/facets.ts:375 and web/src/lib/insights.ts:78 — two exported functions both named `categoryLabel`, both over `CATEGORY_LABELS`, with different fallbacks; filterSections.ts:12 imports the first, routes/insights/*/+page.server.ts:3 the second
- web/src/lib/headerScope.ts:24 — a fourth fallback, `titleCase`, first-character-only again

**Verifier.** Every citation checks out and the divergence is user-visible. labels.ts:1-6 declares itself the single source 'shared by … enrichment.ts … and facets.ts'. enrichment.ts:47-50 humanize uppercases only the first character; facets.ts:305-310 humanize title-cases every word; insights.ts:79 uses a third regex fallback over its OWN CATEGORY_LABELS map (insights.ts:15). CATEGORY_VALUES (contracts.ts:1020) has 37 codes and labels.ts:50-61 overrides only 10, so seven multi-word codes actually diverge — network_engineering, engineering_design, business_analysis, solutions_engineering, developer_relations, technical_writing, customer_success render as 'Network engineering' in the detail-page Category row (enrichment.ts:189 link(...) → label() → humanize) and 'Network Engineering' in the filter panel (facets.ts:363 options(CATEGORY_VALUES, CATEGORY_LABELS)); 'fullstack' is a third split ('Full-Stack' in insights, 'Fullstack' elsewhere). Relocation is the cleanest instance: every other facets.ts vocabulary imports its overrides from labels.ts (WORK_MODE_LABELS, SENIORITY_LABELS, EMPLOYMENT_LABELS, ENGLISH_LEVEL_LABELS, COMPANY_TYPE_LABELS at facets.ts:354-362) — RELOCATION at facets.ts:358 is the one that inlines its own, with not_supported:'None' against enrichment.ts:31's not_supported:'Not supported'. Two exported categoryLabel functions confirmed at facets.ts:375 (imported by filterSections.ts:12, ProfileForm.svelte:6) and insights.ts:78 (imported by the three routes/insights/*/+page.server.ts:3).

#### 37. JobDrawer re-implements two lib modules whose stated job is to be the single home for exactly that code

`jobdrawer-reimplements-shared-modules` · reuse · severity **medium** · effort **S** · verdict **confirmed**

**Problem.** JobDrawer duplicates the description CSS exactly, so the two blocks are one edit away from disagreeing — and the drawer's comment already points at a file that no longer owns the rule, which is how the next person will miss it. The scroll-lock copy is a live defect: JobDrawer's save/restore is invisible to `scrollLock`'s counter, so if the drawer opens first and the header menu is opened over it, the drawer's unmount writes `overflow` back to `''` while the menu still holds a lock and the background scrolls behind an open overlay.

**Remedy.** Keep the remedy as written (use <JobDescription html={…}/> at JobDrawer.svelte:371 and delete the 417-463 style block; swap the $effect at 127-133 for lockScroll/unlockScroll), but drop the 'live defect' framing — the scroll-lock swap is a consistency fix, not a bug fix, because the z-40 header cannot be reached through the z-50 drawer.

**Evidence.**

- web/src/lib/components/JobDescription.svelte:1 — header: "Reused by the job page (JobView), the tracker/drawer, and the tailor artifact panel — one home for the CSS so the description reads the same everywhere"
- web/src/lib/components/JobDescription.svelte:17 — the `.job-description` style block; `diff` against web/src/lib/components/JobDrawer.svelte:420 is empty — 44 lines byte-identical
- web/src/lib/components/JobDrawer.svelte:371 — inlines `<div class="job-description text-sm leading-relaxed">{@html item.job.description}</div>` instead of `<JobDescription html={…} />`; only JobView.svelte:452 and lib/tailor/ArtifactPanel.svelte:207 actually import the component
- web/src/lib/components/JobDrawer.svelte:419 — the copy's comment says "Styles mirror JobView's .job-description", but JobView no longer holds those styles; they were extracted to JobDescription.svelte
- web/src/lib/scrollLock.ts:12 — a reference-counted body lock, documented as "the body only unlocks once every requester has released"
- web/src/lib/components/JobDrawer.svelte:127 — hand-rolls the lock (`const prev = document.body.style.overflow; … = 'hidden'; return () => … = prev`), bypassing the refcount that HeaderSearch.svelte:7 and HeaderMenu.svelte:33 use

**Verifier.** The duplication half is exact and contradicts a documented boundary. `diff <(sed -n '420,463p' JobDrawer.svelte) <(sed -n '17,60p' JobDescription.svelte)` is empty — 44 byte-identical lines. JobDrawer.svelte:371 inlines the same `<div class="job-description text-sm leading-relaxed">{@html …}</div>` that JobDescription.svelte:11 owns, and JobDrawer.svelte:419's comment 'Styles mirror JobView's .job-description' is stale: JobView.svelte has no .job-description rule left (grep finds none), it renders <JobDescription> at line 452. Strongest part: JobDescription.svelte:2-4 documents itself as 'Reused by the job page (JobView), the tracker/drawer, and the tailor artifact panel — one home for the CSS' while the drawer is the one caller that does not import it (only JobView.svelte:16 and tailor/ArtifactPanel.svelte:17 do). The scrollLock half is a real bypass (JobDrawer.svelte:127-133 writes document.body.style.overflow directly while HeaderSearch.svelte:98 and HeaderMenu.svelte:106 use the refcount) but the claimed live defect is NOT reachable: JobDrawer.svelte:158 is `fixed inset-0 z-50` and TopBar.svelte:80 is `sticky top-0 z-40`, so the header menu cannot be opened over an open drawer, and the reverse order is equally blocked by the menu's own overlay.

#### ✅ 38. savedJobs / viewedJobs / dismissedJobs are three copies of one class differing only in which endpoint they call

`three-identical-slug-set-stores` · simplicity · severity **low** · effort **S** · verdict **confirmed**

**Problem.** 196 lines across three files express one idea — a per-user set of job slugs, loaded once, reset on sign-out. The bodies are identical apart from the API method name, so a fix to the reactive-set handling (the `SvelteSet`-not-`Set` subtlety each file re-explains in its own comment) has to be applied three times or it is applied once and the other two silently keep the old behaviour. The next tracking set (there is already a fourth interaction axis in `api.listMyJobs('board')`) will be a fourth copy.

**Remedy.** Same shape as proposed — one SlugSet extends UserResource<string[]> taking the loader in its constructor — but keep viewedJobs' add-only guarantee (viewedJobs.svelte.ts has no unmark by design: 'a view is only ever added'), e.g. by exposing unmark on a narrow subclass or a separate append-only type, not on the shared base. Exporting the three instances and deleting the twelve free-function wrappers is the right second half.

**Evidence.**

- web/src/lib/savedJobs.svelte.ts:19 — `class SavedJobs extends UserResource<string[]>`: `#slugs = $state(new SvelteSet<string>())`, `has`, `mark`, `unmark`, `load() { return api.listSavedSlugs() }`, `apply`, `clearState`
- web/src/lib/dismissedJobs.svelte.ts:18 — `class DismissedJobs` with the identical body; only `load()` differs (`api.listDismissedSlugs()`)
- web/src/lib/viewedJobs.svelte.ts:15 — `class ViewedJobs`, same body minus `unmark` (a view is add-only)
- web/src/lib/userResource.svelte.ts:23 — `UserResource<T>` already owns the load-once/generation-guard/reset half, so the copied part is only the SvelteSet plumbing
- web/src/lib/savedJobs.svelte.ts:54,58,62,68 — a third copy of the free-function façade (`isSaved`/`markSaved`/`markUnsaved`/`ensureSavedLoaded`), mirrored at viewedJobs.svelte.ts:50-57 and dismissedJobs.svelte.ts:53-69

**Verifier.** Verified line by line. savedJobs.svelte.ts:19-52 and dismissedJobs.svelte.ts:18-51 are identical class bodies modulo the load() method name (api.listSavedSlugs vs api.listDismissedSlugs) and comment wording; viewedJobs.svelte.ts:15-43 is the same minus unmark. Each restates the same non-obvious invariant in its own words ('SvelteSet (not a plain Set): a plain Set in $state is not deeply reactive…' at savedJobs:20, dismissedJobs:19, viewedJobs:16), and the files even document the relationship instead of factoring it ('Sibling of viewedJobs.svelte.ts', 'Sibling of savedJobs.svelte.ts: same two-way toggle shape, different mark'). userResource.svelte.ts:22-82 already owns the load-once/generation-guard/reset half, so only the SvelteSet plumbing is copied. The 196-line total and the triple free-function façade (savedJobs:56-70, viewedJobs:47-57, dismissedJobs:55-69, consumed under three different naming families by JobRow.svelte:23-25) are both as stated. The remedy is proportionate — one more concrete subclass of an existing base, parameterised by its loader, not a framework. Two caveats: the 'fourth axis in api.listMyJobs("board")' is speculation (that returns job rows, not a slug set), and nothing is broken today, so this is tidiness rather than a live hazard.

#### ✅ 39. The mail feature's wire shapes are split across api.ts and types.ts with no rule, and the same email row is modelled twice

`mail-wire-shapes-split-across-api-and-types` · boundary · severity **low** · effort **M** · verdict **overstated**

**Problem.** There is no rule deciding whether a mail wire type lands in types.ts or in api.ts, so one feature's shapes straddle both and consumers import types from the transport module. The split has a downstream cost: because `ApplicationEmail` is a near-twin of `InboxMessage` rather than the same type, JobDrawer could not reuse InboxView's message row and copied it — avatar, name, timestamp, subject, status chip and the sandboxed body iframe — and the copies have already drifted (`h-9 w-9` vs `size-9`, `flex-1` vs `h-96`, raw span vs `Badge`). Three renderings of one status chip in two files is the visible symptom.

**Remedy.** Drop the mail.ts reshuffle and the unified EmailSummary — an EmailSummary with optional external_id/snippet would stop mirroring two deliberately different server structs, which is exactly what types.ts:1 says it must not do. Also drop the EmailRow.svelte extraction: over two call sites it would need six conditional props (unread dot, snippet, link chip, selection vs expansion, wrapper) to serve a master-list row and an accordion header. Keep only the one real residual: the status chip is rendered three ways in two files — hand-rolled at InboxView.svelte:642 and JobDrawer.svelte:331 ('inline-block rounded border px-1.5 text-[10px] leading-4') and as <Badge variant="outline"> at InboxView.svelte:722. Pick one rendering (Badge, since it is already a 12-site DS adopter and text-[10px] is an arbitrary value the web token ratchet counts) and use it in all three places.

**Evidence.**

- web/src/lib/types.ts:1 — "Wire types mirroring the backend JSON"; web/src/lib/api.ts:1 — "The only module that knows the API base URL and wire shapes" — two files each claiming the wire shapes
- web/src/lib/types.ts:461 — `EmailLinking` (the classification/link overlay); web/src/lib/api.ts:224 — `InboxMessage extends EmailLinking`, i.e. the inheritance crosses the module boundary in the wrong direction (transport extends domain)
- web/src/lib/types.ts:471 — `ApplicationEmail` is `InboxMessage` minus `external_id`/`snippet`, and hand-inlines two of `EmailLinking`'s six fields (`status_signal`, `link_source`) instead of extending it
- web/src/lib/api.ts:204,213,221,237 — `GmailStatus`, `MailboxStatus`, `InboxSource`, `EmailBody` live in the transport module; `TrackedApplication` (types.ts:484) and `FollowUpDraft` (types.ts:504) live in the other one
- web/src/lib/components/JobDrawer.svelte:15,20 — one component importing `EmailBody` from `$lib/api` and `MyJob, ApplicationEmail` from `$lib/types` for the same feature
- web/src/lib/cv.ts:1 — the counter-example already in the repo: "The server owns the Document wire shape (generated into contracts.ts); this module adds the list/detail response shapes" — one feature module owning its own wire types plus its pure helpers
- web/src/lib/components/InboxView.svelte:642 vs web/src/lib/components/JobDrawer.svelte:331 — the same status chip markup copied (`inline-block rounded border px-1.5 text-[10px] leading-4 {statusClass(...)}`), while InboxView.svelte:722 renders the same chip as `<Badge variant="outline">`; the sandboxed body iframe is likewise copied (InboxView.svelte:771 vs JobDrawer.svelte:345)

**Verifier.** The observations are accurate but the diagnosis is refuted by the backend. api.ts:204/213/221/224/237 and types.ts:462/472/485/505 are where claimed (±1 line). But 'the same email row is modelled twice' is not a frontend defect: the server models it twice on purpose — internal/handler/inbox_linking.go:15 `applicationEmail` is a FLAT struct with only status_signal + link_source, while internal/handler/inbox.go:26 `emailLinking` is embedded by inboxMessage/emailBody. types.ts:1 states its job is 'Wire types mirroring the backend JSON', so ApplicationEmail hand-listing two fields instead of `extends EmailLinking` is correct — extending it would advertise linked_slug/linked_company/suggested_* that GET /me/tracking/:slug never sends. 'Transport extends domain, the wrong direction' is invented: types.ts is not a domain module, it is the other wire-shape module, and api.ts already declares 13 endpoint-shaped types (Slice, SitemapEntry, InsightRole/Skill/SalaryBand/Company, JobCopy, MyJobsFilter) that insights.ts:7 imports the same way — so the api.ts/types.ts line is roughly 'endpoint response shape' vs 'model mirror', imperfect but not absent. The causal claim ('because the types differ, JobDrawer could not reuse InboxView's row') is not supported: InboxView.svelte:609-655 is a selectable master-list item (aria-current, unread dot, snippet, linked/suggested indicator) and JobDrawer.svelte:312-336 is an accordion header (aria-expanded, no dot, no snippet, no link chip) — different affordances, not a copy blocked by typing. The shared decisions here are ALREADY extracted (emailStatus.ts statusLabel/statusClass, avatar.ts avatarInitials/avatarColor); what remains duplicated is layout markup that legitimately differs, and the iframes differ too (JobDrawer.svelte:345 `h-96 w-full` vs InboxView.svelte:769 `min-h-0 w-full flex-1 rounded-md border`).

#### ✅ 40. Six independent tab-strip implementations coexist while the design system's Tabs primitive has zero call sites

`six-tab-strips-ds-tabs-unused` · reuse · severity **low** · effort **L** · verdict **overstated**

**Problem.** One visual/behavioural decision — how a tab strip looks and how arrows move through it — is implemented six times. Two of the copies (JobRelated, ReferralsView) have already drifted in padding and font-weight from TabRow's underline strip, and both fail the ARIA promise TabRow's own comment states (`role="tablist"` without roving tabindex means the group is announced as one widget but cannot be stepped through). JobDrawer's `tabClass` is a hand-transcription of the design system's `tabsTriggerVariants` strings, so a token change in the package silently leaves that drawer behind. Meanwhile the primitive that exists for this is one of the eleven the design-system AGENTS.md names as unreachable.

**Remedy.** Do not move TabRow into the package or grow Tabs a variant/manual-activation/link-tab matrix — tabs.svelte:70-91 welds the list and the panel into one 'flex flex-col gap-2' wrapper, which JobDrawer's fixed-header/scrolling-body layout and the two routed anchor-based /my/* strips structurally cannot use, and ReferralsView needs a snippet label for its count badge. Proportionate app-side fixes only: (1) replace JobRelated.svelte:28-48's inline strip with the existing TabRow (drop-in, and it gains the roving tabindex it never had); (2) add use:tablist to ReferralsView.svelte:140 so its role="tablist" stops lying; (3) hoist the one byte-identical tabClass shared by my/tracking/+layout.svelte:21 and my/activity/+layout.svelte:31 into a single exported helper next to actions/tablist.ts. Leave JobDrawer's pill and the DS Tabs baseline alone.

**Evidence.**

- design-system/src/tabs.svelte:8 — `tabsTriggerVariants` tv(): base `rounded-md px-3 py-1 text-sm font-medium…`, active `bg-card text-foreground shadow-sm`, inactive `text-muted-foreground hover:text-foreground`, plus roving tabindex + Arrow keys (lines 61-88)
- design-system/scripts/adoption-baseline.json:15 — `"Tabs": 0` (no web/src file imports it)
- web/src/lib/components/TabRow.svelte:131 — a second full tablist: own roving tabindex, Arrow/Home/End (lines 99-124), scroll-fade mask; used by exactly 2 files
- web/src/lib/actions/tablist.ts:35 — a third implementation of the same keyboard behaviour, as a Svelte action, used by 3 files
- web/src/lib/components/JobDrawer.svelte:143 — `tabClass()` hand-copies the DS pill variants (`rounded-full px-4 py-1.5` / active `bg-card font-medium text-foreground shadow-sm`) over a `role="tablist"` at line 230
- web/src/lib/components/JobRelated.svelte:28 — `tabClass()` copies TabRow's underline visual but drifted (`pb-2` vs `pb-2.5`) and the buttons carry no `role` at all (lines 40, 45)
- web/src/lib/components/ReferralsView.svelte:140 — `role="tablist"`/`role="tab"` with the underline visual again (drifted to `px-3 py-2.5 text-sm font-semibold`) and no keyboard handling
- web/src/routes/my/tracking/+layout.svelte:21 and web/src/routes/my/activity/+layout.svelte:31 — byte-identical `tabClass()` helpers

**Verifier.** The census is real but two load-bearing claims are false. Real: design-system/scripts/adoption-baseline.json:15 says "Tabs": 0; TabRow.svelte is imported by exactly 2 files (routes/my/profile/+page.svelte:17, ProfileForm.svelte:18); actions/tablist.ts by 3 (JobDrawer.svelte:8, my/activity/+layout.svelte:6, my/tracking/+layout.svelte:6); my/tracking/+layout.svelte:21-27 and my/activity/+layout.svelte:31-37 are byte-identical tabClass helpers; the underline visual exists three times with drift (TabRow.svelte:147 'px-1 pb-2.5 … font-medium', JobRelated.svelte:28-33 'px-1 pb-2 … font-medium', ReferralsView.svelte:147-149 'px-3 py-2.5 … font-semibold'). FALSE: (a) 'JobDrawer's tabClass is a hand-transcription of tabsTriggerVariants' — JobDrawer.svelte:143-149 is 'rounded-full px-4 py-1.5 … transition-colors' against tabs.svelte:9's 'rounded-md px-3 py-1 … transition-all focus-visible:ring-2'; different radius, padding, transition and no focus ring. And the stated failure mode ('a token change in the package silently leaves that drawer behind') is wrong: bg-card/text-foreground are token-backed utilities, so a token value change reaches both. (b) 'both JobRelated and ReferralsView fail the ARIA promise (role="tablist" without roving tabindex)' — JobRelated.svelte:39-48 carries no role at all, so there is no tablist promise to break; only ReferralsView.svelte:140-144 has role=tablist/role=tab with no keyboard handling. Also, the DS's own AGENTS.md documents unused primitives as a tracked, ratcheted state ('names the unused ones every run (eleven of fifteen today)') and reasons case-by-case about legitimate non-migrations ('What Dialog does not cover'), so adoption=0 is not by itself the defect the finding treats it as.

### Cross-cutting duplication sweep

> **Reviewer's note on what is well-factored here.** Genuinely well-factored, and deliberately NOT flagged:\n\n- **The small shared packages are real, not decorative.** `internal/wordmatch` is used by every dictionary that needs whole-word matching (roletag.go:392, classify/classify.go:81 and :121, classify/tech.go:96, classify/nontech.go:231, classify/description.go:103/:119, skilltag/skilltag.go:85/:241, cvmatch/title.go:44, hardconstraint/degrees.go:55) with the two boundary policies factored as `Boundary` funcs — that is exactly the right seam. `internal/normalize.Slug`/`JobSlug` is the only slug generator in the repo. `internal/sources/dates.go` is the single date-parsing home for 186 adapters. `internal/sources/helpers.go` (isRemote, workModeFromRemote, workplaceTypeMode, fetchDetails) is well used, with only a couple of justified adapter-specific variants.\n\n- **The dictionary/facet packages are NOT five copies of one engine.** I checked the hypothesis directly. `internal/classify` uses an ordered `[]aliasEntry` first-match walk (classify.go:119 `matchOrdered`); `internal/skilltag` uses a word-token map plus a separate phrase-regex pass over stripped markup; `internal/location`, `internal/roletag` and `internal/collections` each have a genuinely different lookup shape. The one thing they DO share — token-boundary matching — is already factored into `wordmatch`. The 1185-line `skilltag/dictionaries.go` and 632-line `classify/dictionaries.go` are data, not logic.\n\n- **`internal/jobderive` + `internal/job` is a model of the pattern.** `job.New` (job.go:118) is a single guarded construction door with unexported state, and every write path (pipeline.go:574, moderation.go:239, linkimport.go:178, tg-extract) goes through it. `search.JobDocument` embeds `jobview.Job` rather than re-projecting.\n\n- **Considered and dropped: `cmd/backfill-derive` discarding structured-source facets.** `deriveRow` (main.go:146) rebuilds a `jobderive.Input` without Seniority/Category/Skills/EmploymentType/ExperienceYearsMin, so a backfill run overwrites adapter-supplied structured values with dictionary output. That looked like a strong finding until I read the command's own doc comment (cmd/backfill-derive/main.go:30-39), which states the behaviour and the reasoning explicitly. Documented deliberate decision — not a defect.\n\n- **Considered and dropped: six copies of `<[^>]*>` tag stripping** (lang.go:25, skilltag.go:34, jobhash/rolefingerprint.go:40, mailclassify/keyword.go:15, collections/nlsponsor.go:22, harvest-ats/resolve.go:80). The entity-handling does differ (only jobhash calls `html.UnescapeString`), but I ran the skill tagger against entity-encoded descriptions and could not produce a divergent result, so I have no failure scenario. Reported as a note rather than a finding.\n\n- **Not flagged: the four `flexdecode.go` files** (atscheck, mailclassify, matchanalysis, telegram). They look like copy-paste but each is a per-type `UnmarshalJSON` alias shim over `internal/flexjson` — that is the idiomatic Go form and cannot be shared further without reflection.\n\n- **Not flagged: `internal/config`'s per-worker structs** (Enrich/Embed/Credits/Reindex). The split is deliberate and documented; my finding is scoped narrowly to the LLM block that is genuinely modelled three times.

#### 41. CATEGORY_LABELS and SENIORITY_LABELS are declared twice in the web app, against labels.ts's own stated "keep ONE map" rule, and have already drifted

`duplicate-facet-label-maps-web` · reuse · severity **medium** · effort **S** · verdict **confirmed**

**Problem.** The same facet code renders under different names depending on which page the user is on. `ai_engineering` is "AI Engineer" in the filter panel and job detail (labels.ts) and "AI Engineering" on /insights. Codes that are in neither map get three different fallbacks: `network_engineering` becomes "Network engineering" on a job detail page (enrichment.ts humanize), "Network Engineering" in the filter panel (facets.ts humanize), and "Network Engineering" on /insights (its own inline regex). Because /insights pages are SEO surfaces whose titles and auto-intro sentences are built from `categoryLabel`, the mismatch is indexed. labels.ts states this exact class of drift as the reason it exists, so the second copy contradicts a documented boundary.

**Remedy.** Do the map half only: delete insights.ts's CATEGORY_LABELS/SENIORITY_LABELS, import them from $lib/labels, and add to labels.ts the handful of insights entries whose label differs from the fallback (fullstack → 'Full-Stack', plus the seniority set and the '' all-levels key insights needs). Leave the humanize unification out: enrichment.ts:46-49's sentence-case is what its own doc comment's example specifies ('data_engineering' → 'Data engineering'), so folding it into facets.ts's title-case is a visual change to job-detail chips, not a drift fix — and insights.ts:79's inline regex already produces the same output as facets.ts's humanize.

**Evidence.**

- web/src/lib/labels.ts:1 — "Single source of display labels for the closed-vocabulary facet codes … Keeping ONE map prevents the drift that previously left stale region codes and inconsistent casing in two places."
- web/src/lib/labels.ts:50 — `export const CATEGORY_LABELS` (10 entries); labels.ts:52 has `ai_engineering: 'AI Engineer'`
- web/src/lib/insights.ts:15 — a second `export const CATEGORY_LABELS` (35 entries); insights.ts:27 has `ai_engineering: 'AI Engineering'`
- web/src/lib/labels.ts:35 — `export const SENIORITY_LABELS: Record<string,string> = { c_level: 'C-level' }`
- web/src/lib/insights.ts:66 — a second `export const SENIORITY_LABELS` with all eight grades spelled out
- web/src/lib/facets.ts:305 — `humanize()` title-cases every underscore-separated word
- web/src/lib/enrichment.ts:47 — a different `humanize()` that capitalizes only the first letter
- web/src/lib/insights.ts:79 — a third, inline fallback: `category.replace(/_/g,' ').replace(/\b\w/g, c => c.toUpperCase())`

**Verifier.** Verified line-for-line. web/src/lib/labels.ts:1-6 states 'Keeping ONE map prevents the drift...'; labels.ts:50 declares CATEGORY_LABELS with `ai_engineering: 'AI Engineer'` at :52 and SENIORITY_LABELS at :35 (`{ c_level: 'C-level' }`). web/src/lib/insights.ts:15 declares a second CATEGORY_LABELS (35 entries) with `ai_engineering: 'AI Engineering'` at :27, and a second SENIORITY_LABELS at :66. The two maps really do feed different surfaces: facets.ts:362 `options(CATEGORY_VALUES, CATEGORY_LABELS)` and :376 `sourceLabel`-style lookup, enrichment.ts:189 `link('Category','category',e.category,CATEGORY_LABELS)` — vs insights.ts:79 categoryLabel, imported only by src/routes/insights/**/+page.server.ts and sitemap-insights.xml/+server.ts. So the same facet code renders 'AI Engineer' in the filter panel and 'AI Engineering' on an SEO page, and nothing keeps them in sync. This is a duplicated decision contradicting its own module's documented rule, and it has already drifted.

#### 42. A company has no wire-shape owner: the generated sqlc row IS the public JSON, and the Meili path fabricates a fake sqlc row to match it

`company-has-no-wire-projection` · boundary · severity **medium** · effort **M** · verdict **overstated**

**Problem.** Jobs have `internal/jobview` as the one owned public projection, and the search document embeds it. Companies have nothing equivalent: three types model the entity (`db.Company`/`db.ListCompaniesRow`, `handler.companyView`, `search.CompanyDocument`), the persistence driver's `pgtype.Text`/`pgtype.Int4` reach the JSON contract, and the two list backends are kept identical only by a hand-written adapter that converts plain strings back into pgtypes. `make sqlc` is therefore an API-changing operation for /companies: adding a nullable column to the ListCompanies query changes the response body with no handler edit, and the Meili branch silently omits it because `companyRowFromDoc` sets fields one by one. The same request then returns different bodies depending on whether Meilisearch is reachable, since the Meili path is a fallback-on-error branch.

**Remedy.** Don't stand up a new internal/companyview package mirroring jobview (companies have one 6-field list row and one detail view; jobview earns its package because it is shared by list/detail/search/index and hydrates a domain aggregate). Proportionate fix inside internal/handler: declare a `companyListItem` with plain Go types (`Tagline, HqCountry *string`), give it `fromListRow(db.ListCompaniesRow)` and `fromDocument(search.CompanyDocument)`, and have ListCompanies serve `[]companyListItem` from both branches. That deletes pgText and the fake-db-row adapter and makes a new sqlc column a deliberate edit. Retyping companyView's pgtype fields via internal/pgconv is a separate, cheap follow-up.

**Evidence.**

- internal/handler/companies.go:230 — `return listResponse(c, companies, total, limit, offset)` serves `[]db.ListCompaniesRow` verbatim as the /api/v1/companies body
- internal/db/companies.sql.go:307 — `type ListCompaniesRow struct { … Tagline pgtype.Text `json:"tagline"` … HqCountry pgtype.Text `json:"hq_country"` }`; the sqlc-generated json tags are the public contract
- internal/handler/companies.go:74 — `companyView` (the detail projection) types YearFounded/EmployeeCount as `pgtype.Int4` and HqCountry/OrganizationType/Tagline/Maturity as `pgtype.Text`
- internal/handler/companies.go:357 — `companyRowFromDoc` builds a `db.ListCompaniesRow` out of a search hit "so the Meili path is byte-for-byte compatible with the Postgres path"
- internal/handler/companies.go:376 — `pgText(s string) pgtype.Text` re-wraps a plain string INTO a driver type purely to make JSON serialization match
- internal/search/company.go:37 — `CompanyDocument` models the same entity with plain `string` fields (its FromCompany unwraps the pgtypes)
- internal/search/document.go:34 — the job side does the opposite: `JobDocument` embeds `jobview.Job`, the single owned wire type
- internal/jobview/AGENTS.md:1 — "The single JSON representation of a job served by the list, detail, and search endpoints and stored in the search index … so the API surfaces cannot drift apart"

**Verifier.** Every citation is real: internal/handler/companies.go:230 does `return listResponse(c, companies, total, limit, offset)` with `companies []db.ListCompaniesRow`; internal/db/companies.sql.go:307-314 is the generated struct whose json tags are the /companies body; companyView (companies.go:69-96) really types YearFounded/EmployeeCount as pgtype.Int4 and HqCountry/OrganizationType/Tagline/Maturity as pgtype.Text; companyRowFromDoc is at :357 and pgText at :375; search/company.go:36-56 models the same entity with plain strings and document.go:32-34 shows JobDocument embedding jobview.Job. So the ownership asymmetry is real and the `make sqlc`-changes-the-API observation is correct.

But two of the claims do not survive. (1) 'The same request then returns different bodies depending on whether Meilisearch is reachable' is false today: companyRowFromDoc sets all six ListCompaniesRow fields (Slug, Name, JobCount, Tagline, Industries, HqCountry) and normalizes nil industries to []; pgText is the exact inverse of FromCompany's unwrap. The drift is latent, not present. (2) The pgtype leak is not visible on the wire — pgx v5.10.0's pgtype.Text/Int4 implement MarshalJSON (~/go/pkg/mod/.../pgtype/text.go:61), so the JSON is a bare string or null. That leaves a type-ownership smell with a latent parity trap, not something that actively causes bugs or blocks work — 'high' is not earned.

#### 43. JobCopies hand-rolls limit/offset parsing and the list envelope instead of using pageParamsMax/listResponse, dropping the int32 clamp those helpers exist for

`copies-endpoint-bypasses-page-helpers` · reuse · severity **low** · effort **S** · verdict **confirmed**

**Problem.** `GET /api/v1/jobs/:slug/copies?offset=3000000000` clamps to 3000000000 (Fiber's QueryInt is a plain strconv.Atoi, so on 64-bit it parses fine), then `int32(3000000000)` wraps to -1294967296 and Postgres rejects the negative OFFSET — the endpoint 500s where every other list endpoint returns an empty page. The shared helper was written with a doc comment naming exactly this overflow, and this call site re-implements it without the clamp. Separately the endpoint's `meta` carries only `total`, so a client reading the documented `{data, meta:{total,limit,offset}}` envelope gets a different shape here than from /jobs, /companies, /search.

**Remedy.** Minimal: clamp the offset in copies.go the way the helper does. If you want the helper, add `pageParamsMax(c, defaultLimit, ceiling)` (or a variadic default) and call it from copies.go:39-40 — but leave similar.go:32 alone: it parses no offset at all, so it is one clamped limit line, not a third copy of the same decision. Changing the copies meta envelope is optional and unrelated.

**Evidence.**

- internal/handler/handler.go:119 — "pageParams reads and clamps the shared limit/offset pagination query params. The offset is clamped into int32 range because the column binds as a Postgres int4, and an unbounded query value would otherwise overflow on the conversion."
- internal/handler/handler.go:130 — `offset = min(max(c.QueryInt("offset", 0), 0), math.MaxInt32)`
- internal/handler/copies.go:40 — `offset := max(c.QueryInt("offset", 0), 0)` — no MaxInt32 clamp
- internal/handler/copies.go:44 — `RowOffset: int32(offset)` feeds that unclamped value straight into the int4 param
- internal/db/jobs.sql.go:1464 — `RowOffset int32` on ListRoleClusterCopiesParams
- internal/handler/copies.go:66 — `c.JSON(fiber.Map{"data": copies, "meta": fiber.Map{"total": total}})` omits the limit/offset echo
- internal/handler/handler.go:139 — `listResponse` is documented as "the single source of the list wire shape, so the jobs/companies/search list endpoints cannot drift from one another"
- internal/handler/similar.go:32 — the same inline `min(max(c.QueryInt("limit", …),1), max…)` idiom, a third copy

**Verifier.** The defect is real and the citations are near-exact (only handler.go:130 vs :131 for the offset line is off by one). internal/handler/handler.go:119-124 documents the int32 clamp as pageParams' reason for existing; handler.go:131 does `offset = min(max(c.QueryInt("offset",0),0), math.MaxInt32)`. internal/handler/copies.go:40 is `offset := max(c.QueryInt("offset", 0), 0)` with no ceiling, and :44 feeds it to `RowOffset: int32(offset)`; db.ListRoleClusterCopiesParams.RowOffset is int32 (internal/db/jobs.sql.go:1464) and the query ends `LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset)` (internal/db/queries/jobs.sql), so a wrapped-negative offset is a Postgres error → 500 on a public endpoint. Fiber v2's QueryInt is strconv.Atoi, so offset=3000000000 does parse on 64-bit. Downgraded to low: it takes a hostile/absurd query value to trigger, nothing is corrupted, and the second half of the finding is weak — copies' meta carries the whole-cluster COUNT(*) OVER (copies.go:60-65), a different quantity from a filtered list total, and JobCopies' own doc comment already declares its response shape.

#### 44. internal/handler never imports internal/pgconv and re-declares two of its converters locally

`handler-reimplements-pgconv` · reuse · severity **low** · effort **S** · verdict **confirmed**

**Problem.** `internal/pgconv` is imported by 17 packages and states as its purpose that these conversions live in one place. `internal/handler` — the largest package in the repo — imports it in zero non-test files and instead carries private twins with different names, so a reader grepping for the conversion finds three spellings and cannot tell whether they agree. It is small in isolation, but it is the same habit that produced `pgText` in companies.go and it is what keeps the pgtype vocabulary alive inside the transport layer.

**Remedy.** Delete `tsFromPtr` (match_analysis.go:427) and `int4Ptr` (hardconstraint_inputs.go:138); import `internal/pgconv` and call `pgconv.Timestamptz` and `pgconv.IntPtr` at their two call sites (match_analysis.go:282 and the hardconstraint input builder).

**Evidence.**

- internal/pgconv/pgconv.go:1 — "Repositories map their domain types across the persistence boundary through these helpers so the nil<->NULL and pgtype<->Go conversions live in exactly one place instead of being re-declared in every package that touches the database."
- internal/pgconv/pgconv.go:27 — `func Timestamptz(t *time.Time) pgtype.Timestamptz`
- internal/handler/match_analysis.go:427 — `func tsFromPtr(t *time.Time) pgtype.Timestamptz` — a character-for-character duplicate of it
- internal/pgconv/pgconv.go:45 — `func IntPtr(n pgtype.Int4) *int`
- internal/handler/hardconstraint_inputs.go:138 — `func int4Ptr(v pgtype.Int4) *int` — the same function under a different name
- internal/jobview/jobview.go:363 — the sibling read-path package does use `pgconv.Timestamptz`/`pgconv.TimePtr`

**Verifier.** Verified exactly. internal/pgconv/pgconv.go:1-5 states the one-place rule; :27 `func Timestamptz(t *time.Time) pgtype.Timestamptz` and :45 `func IntPtr(n pgtype.Int4) *int`. internal/handler/match_analysis.go:427-432 is a body-identical `tsFromPtr`, used at :282 (`CvUploadedAt: tsFromPtr(cvUploadedAt)`), and internal/handler/hardconstraint_inputs.go:138-143 is a body-identical `int4Ptr`, used at :78 (`ExperienceYearsMin: int4Ptr(job.ExperienceYearsMin)`). `grep -n pgconv internal/handler/*.go` returns nothing — the largest package in the repo genuinely never imports it, while the sibling read-path package does (internal/jobview/jobview.go:363 uses pgconv.Timestamptz and pgconv.TimePtr). Low severity is correctly stated; the remedy is a two-function deletion and an import, with no new abstraction.

#### ✅ 45. The six LLM/Langfuse env values are modelled by three separate structs and copied field-by-field at seven call sites; two workers import the enrichment config just to reach them

`llm-settings-modelled-three-times` · reuse · severity **low** · effort **S** · verdict **overstated**

**Problem.** Adding or renaming one LLM setting means editing three struct definitions, two env loaders, and seven copy literals. The redundancy has already produced dead weight: `config.Enrich.LangfuseEnabled` is unreachable now that `llm.NewClient` gates tracing itself. It also produces false coupling — `cmd/tg-extract` and `cmd/classify-mail` call `config.LoadEnrich()` purely for the six LLM fields (grep shows they touch none of Concurrency/LeaseSeconds/MaxAttempts), so a Telegram extractor and a mail classifier inherit the enrichment worker's ENRICH_* required-env validation and would break if that validation changed.

**Remedy.** Keep llm.Settings exactly as it is. Hoist the six shared fields into one `config.LLM` struct with a single loader, embed it in both Settings (optional) and Enrich (fail-fast, keeping only Concurrency/LeaseSeconds/MaxAttempts of its own), and give it one `func (l LLM) Settings() llm.Settings` so the seven literals become `llm.NewClient(cfg.LLM.Settings(), "source")`. Delete Enrich.LangfuseEnabled and its test. Renaming the loader the two non-enrich workers call (or giving them a LoadLLM) is worth doing only as part of that same edit.

**Evidence.**

- internal/config/config.go:70 — `LLMBaseURL/LLMAPIKey/LLMModel` on Settings, plus `LangfuseBaseURL/PublicKey/SecretKey` at config.go:99, loaded at config.go:192 and config.go:203
- internal/config/enrich.go:14 — `type Enrich struct` re-declares the same six fields (:15-17 and :26-28), loaded from the same env keys at enrich.go:42 and enrich.go:49
- internal/llm/llm.go:121 — `type Settings struct` is the third copy of the same six values
- cmd/server/main.go:143 and cmd/server/main.go:160 — two field-by-field copies into llm.Settings
- cmd/enrich/main.go:34, cmd/tg-extract/main.go:46, cmd/classify-mail/main.go:32, cmd/backfill-resume-structured/main.go:70, cmd/backfill-experience/main.go:205 — five more identical six-line literals
- internal/config/enrich.go:34 — `func (e Enrich) LangfuseEnabled() bool` has no non-test caller; llm.NewClient decides internally via `llm.Settings.Enabled()` (internal/llm/llm.go:147)
- internal/config/embed.go:7 — the codebase's own preferred split: Embed "holds only the queue-drain knobs, mirroring the tuning half of config.Enrich", with the connection settings elsewhere

**Verifier.** Citations all check out: config.go:70 (LLMBaseURL/APIKey/Model), config.go:99 (Langfuse trio), loaded at :192/:203; internal/config/enrich.go:15-17 and :26-28 re-declare the same six and LoadEnrich re-reads the same env keys; the seven `llm.Settings{...}` literals are exactly where claimed (server:143 and :160, enrich:34, tg-extract:46, classify-mail:32, backfill-resume-structured:70, backfill-experience:205); `grep ecfg\.` proves tg-extract and classify-mail touch only the six LLM fields and never Concurrency/LeaseSeconds/MaxAttempts; and LangfuseEnabled (enrich.go:34) has no caller outside enrich_test.go. So there IS a real duplicate: config.Settings and config.Enrich read the same six env vars, and a dead method has already grown out of it. The framing is what is wrong. llm.Settings is not a third copy — internal/llm/llm.go:121-126 documents it as 'the one shape every entrypoint maps its env config into, so client construction and tracing live in exactly one place', the same env-free-library split config/embed.go:44-49 documents for EmbedClient vs search. The two config structs also differ in policy on purpose (config.go:65-69 says the server's LLM is optional and degrades; LoadEnrich fails fast naming every missing var), so this is not one decision written twice so much as two requirement levels over shared keys. Nothing here causes a bug or blocks work — it is edit-in-N-places friction.

#### 46. Two backfill workers hand-roll worker.Bootstrap and duplicate the whole résumé-extractor construction stack

`backfill-workers-bypass-worker-bootstrap` · coupling · severity **low** · effort **M** · verdict **overstated**

**Problem.** `internal/worker` exists precisely so every cron binary gets Sentry init, a SIGTERM-cancellable context, and pool lifecycle in one place; 30+ workers use it and these two do not. A panic or error inside either backfill is invisible to Sentry, and a redeploy's SIGTERM kills them mid-write instead of unwinding, because their root context is `context.Background()`. On top of that, the "blobstore + LLM client + PII detector + resume.Store + Extractor" chain is now assembled three times (two workers plus the server) with the wiring order and the fail-closed PII rule re-stated by hand each time — the backfill-resume-structured copy makes each piece fatal, the backfill-experience copy makes each piece optional, and only comments keep the PII gate consistent.

**Remedy.** Do only the first half: convert both mains to `worker.Main(run)` + `worker.Bootstrap(context.Background())`, which is a drop-in for the config.Load/Connect/defer-Close they hand-roll. Drop the NewStack proposal — a single constructor serving one fail-fast caller, one fail-open caller and the server's pre-built deps would need a policy flag, which is exactly the abstraction-over-two-cases the project's rules forbid.

**Evidence.**

- internal/worker/bootstrap.go:29 — `Bootstrap` is "the setup every standalone worker shares: it initializes error reporting, loads config, derives a signal-cancellable root context, and opens the database pool"
- internal/worker/main.go:23 — `Main(run func() int)` is "the entry wrapper every cron worker uses in place of a bare os.Exit(run())", reporting panics to Sentry
- cmd/backfill-resume-structured/main.go:47 — `cfg := config.Load(); ctx := context.Background()` then `database.Connect` at :50 — no Sentry, no signal context, no `worker.Main`
- cmd/backfill-experience/main.go:62 — the same four lines repeated verbatim (config.Load, context.Background, database.Connect, defer pool.Close)
- cmd/backfill-resume-structured/main.go:57 — blobstore.New{Endpoint,Bucket,AccessKey,SecretKey} → llm.NewClient (:70) → PIIFilterURL guard (:88) → pii.NewHTTPDetector (:91) → resume.New (:93) → resumeextract.NewExtractor (:94)
- cmd/backfill-experience/main.go:192 — `newExtractor(cfg, queries)` reproduces that identical six-step chain at :193-217
- internal/handler/handler.go:259 — the server builds the same pair (`resume.New(cfg.Blob, …)` at :259, `resumeextract.NewExtractor(cfg.LLM.WithTimeout(…), cfg.PIIDetector)` at :274)
- cmd/rollup-views/main.go:45, cmd/enrich/main.go:48, cmd/prune/main.go:124 — ~30 other workers do go through worker.Bootstrap

**Verifier.** Half true, half wrong. Confirmed: cmd/backfill-resume-structured/main.go:47-53 and cmd/backfill-experience/main.go:62-68 both do `config.Load()` + `context.Background()` + `database.Connect` by hand, and `grep -rL 'worker.Bootstrap|worker.Main' cmd/*/main.go` returns only these two plus cmd/server and nine DB-free dev tools (cv-previews, gen-*, harvest-*), so among DB workers they are the outliers against internal/worker/bootstrap.go:23-29's 'every standalone worker'. But the stated consequence is invented: both are documented one-off, operator-run, idempotent backfills ('go run ./cmd/backfill-experience', 'Idempotent by construction'), there is no systemd unit or timer for either anywhere in the repo, so 'a redeploy's SIGTERM kills them mid-write' is speculation. The duplication half is worse. The server does NOT assemble the chain: cmd/server builds blobstore/LLM/PII once into handler.Config, and handler.go:259 / :274 are two isolated constructor calls with already-built deps, separated by unrelated wiring — that is not a third copy of a six-step chain. And the two workers' chains encode deliberately opposite policies, both documented: backfill-resume-structured/main.go:63-90 makes every piece fatal ('a backfill run without it is pointless — require it'), backfill-experience/main.go:190-192 makes every piece optional ('With the S3/LLM/PII settings unconfigured the worker still runs the free pass').

---

## Found while implementing

Findings raised by the work of fixing the ones above, not by the original review pass.
They carry no verifier note — they were established against the code by the implementer.

### ✅ I1. Three test fixtures exercise the AGENT write path with no evidence gate, and one documents the divergence as intentional

`cv-user` · coverage · severity **medium** · effort **M** · found while implementing S5

**Problem.** `cv_tailor_integration_test.go:47`, `assistant_cv_tools_test.go:94` (via
`cvToolsAPI`) and the cases reached through them build a `cvHandlers` struct LITERAL —
bypassing `newCVHandlers` — with a nil evidence gate, then edit as `ActorAgent`. Because
they bypass the constructor, making the gate a construction-time argument (S5) neither
fixes nor breaks them: they keep asserting the behaviour of a configuration production no
longer has. `cv_tailor_integration_test.go:276` states the divergence outright — "the key
edits as the AGENT, so a bullet has to cite banked evidence — except that this fixture
wires no bank, which is 'no gate' rather than 'gate that refuses' (the CLI is not a place
to enforce provenance it cannot query)" — while production attaches a gate to that very
editor and does refuse. One of the two is wrong about the product.

**Remedy.** Decide the product question first: must an API-key caller editing
`PATCH /me/cvs/:id` as the agent cite banked evidence? Production says yes. If that is
right, give those fixtures a real gate and banked fixtures for the cases that expect a
write to land; if it is not, the gate belongs behind an explicit per-actor policy rather
than being attached to every editor the assembly builds. Do not settle it by changing the
fixture comment.

**Evidence.**

- internal/handler/cv_tailor_integration_test.go:47 — `editor: cvedit.NewEditor(cvedit.NewRepository(pool, queries), nil)` inside a struct literal, not `newCVHandlers`
- internal/handler/cv_tailor_integration_test.go:279 — `doBearer(... PATCH ...)` inserting a bullet, expecting 200; `internal/handler/cv_tailor.go:241` makes that `ActorAgent`
- internal/handler/cv_tailor_integration_test.go:276 — the comment recording the divergence as deliberate
- internal/handler/assistant_cv_tools_test.go:94 — `cvToolsAPI` builds a nil-gate editor; its sibling `memRevisions` doc at :112 claims a tool test "exercises the real editor — policy, evidence gate, apply, coalescing"
- verified: the two cases that DO exercise the gate (`TestCVEditWithoutAnyEvidenceIdNamesWhereItGoes`, `TestCVEditWritesACitedBullet`) had to attach a bank by hand; they now construct with one
- verified against the code: with the CV surface assembled through `newCVHandlers` and no gate, an API-key `PATCH` inserting a bullet returned **200** — a new integration test now pins it at 403

### ✅ I2. Autofill has TWO contact sources, not one, and reads only the one that appears second

`extension-autofill` · boundary · severity **medium** · effort **S** · found while implementing #2

**Problem.** Finding #2 asked which CV autofill should pick and demanded a decision about the
user whose CVs are all tailored. Both halves of that framing are wrong, and the production
numbers say so plainly. There are **two** places holding the candidate's contact block —
`cvs.data.header` and `users.resume_structured`, the second seeding the first through
`cv.Seed` — and autofill read only the first. On 2026-08-02: 174 users have a current
structured résumé, **10** have a CV. Among the 17 API-key holders the extension actually
serves, **one** had a CV and seven more had a résumé and got nothing but their account
email. Meanwhile the tailored-only case #2 wanted a policy for has **zero** rows.

So the defect was never "picks the wrong CV". It was "reads one of two sources, and the one
that appears later in a user's life". Fixed in #1478: base CV header → structured résumé
contacts → account email, first source that states a contact answering for the whole block.

Three things worth keeping:

- **The precedence has a reason that is not recency.** The CV header wins because it is the
  copy the candidate authored and the only one the tailoring agent cannot rewrite —
  `cvedit.DefaultPolicy` denies `ActorAgent` the header's name, email, phone and links and
  denies the candidate nothing. Corrected beats derived.
- **Not merging is the load-bearing half.** Since the résumé seeds the CV, a field-by-field
  merge would restore precisely the value the candidate deleted from their CV. The rule that
  matters is "the chosen source answers for the whole block", not the ordering.
- **The measurement replaced the question.** #2's blocking question ("fall back, or accept
  the empty header?") evaporated against `count(*) = 0`. That is the third time on this list
  — after #36 and #16 — that a number closed an argument the finding framed as a decision.

**Evidence.**

- internal/handler/autofill_profile.go:74-91 (before) — the two `pool.QueryRow` calls; the second `ORDER BY updated_at DESC LIMIT 1` over every CV
- internal/cv/seed.go:18-25 — `Seed` maps the structure's five contact fields into `cv.Header` one-for-one, which is why the two sources hold the same fields
- internal/cvedit/policy.go:33-45 — the four header paths denied to `ActorAgent`
- internal/resume/resume.go:239-253 — `Structured` applies the stamp-freshness rule; production has 174 of 174 stamps current
- prod `hire` 2026-08-02: `users` 408 · with résumé 174 · with structure 174 · with base CV 10 · tailored-without-base 0; key holders 17 · with base CV 1 · structure-only 7

### I3. The command list in AGENTS.md is not a list of where the tagged tests live

`repo-docs` · coverage · severity **low** · effort **XS** · found while implementing #2

**Problem.** `go test ./...` compiles no `//go:build integration` file. AGENTS.md's command
block named exactly one tagged run — `go test -tags=integration ./internal/db/`, labelled
"queue integration tests" — and I ran it, correctly, and shipped a broken build. There are
**152** tagged files across **13** packages; `internal/handler` holds **65**, and they call
unexported constructors. Changing `newCVHandlers`'s signature passed `go build`, `go vet`,
`go test ./...` and the one documented tagged run, then failed CI on `not enough arguments`.

The label was accurate and the inference from it was not: a command list is not an
inventory. Fixed in #1481 by adding the guard that costs seconds and needs no Docker —
`go vet -tags=integration ./...` — plus the sentence saying why it is there.

This is the same shape as the seven closed before it, with the roles swapped: not a document
claiming a mechanism the code lacks, but a document whose accurate sentence I read as a
claim it never made.

---

## Method

- Nine area reviewers, one per slice of the repo, each applying all four lenses and required to
  cite `file:line` for every claim (two sites minimum for a reuse or coupling claim).
- One skeptic per area, instructed to refute by default: re-open every citation, distinguish a
  duplicated *decision* from merely similar-looking code, check each finding against the
  package's own `AGENTS.md`, and reject any remedy that builds a framework, a one-implementation
  interface or a generic abstraction over two cases.
- One synthesis pass to merge cross-area duplicates, group into themes and rank by
  cost-over-effort.
- Reviewers read only the main checkout; `.claude/worktrees/**`, `node_modules` and generated
  files (`internal/db/*.sql.go`, `web/src/lib/generated/contracts.ts`) were excluded from style
  review but included when judging how their types are consumed.

Findings are a snapshot of the tree at the date in the title. Line numbers drift; the `file:line`
citations are a starting point, not a permanent address.
