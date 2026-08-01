## Context

`cmd/harvest-boards` already turns a candidate slug list into live-validated entries in
`sources/<provider>.yml`: it de-duplicates against the file, probes each survivor through one
of 32 per-platform probers, and appends what answers with jobs. It reads two seed shapes — a
bare `["slug", …]` array and a richer `[{board, company}, …]` — and it already threads the
seed's `company` through `resolveCandidates` into `probeAll` as `companyByBoard`.

What it does with that name is the gap. `chooseCompany` (`cmd/harvest-boards/seed.go:43`)
prefers the platform's reported name whenever it is not just the slug echoed back, and falls
back to the seed's. The seed name is a *label of last resort*, never a check. So a candidate
that resolves to a live board belonging to someone else is kept and labelled with the wrong
employer's name — the failure that put iCIMS `prequel` under A. C. Coy's tenant in July and
forced a revert.

What has never existed is a way to decide *which* companies to chase. Every seed so far came
from outside: a Common Crawl dump, a universities directory, a scraped company list. The
catalogue itself knows which companies we hold only as aggregator mirrors, and it is the
best worklist available — 8,571 companies across the eight remote-jobs aggregators, measured
2026-08-01.

A July attempt at this went by way of each company's website (`slug → .com` → careers page →
`atsdetect`) and stalled on website resolution, not on ATS detection. Probing platform APIs
with a name-derived slug skips the website entirely. Measured against 12 platform APIs on 80
randomly sampled uncovered himalayas companies: 12 live boards (15%), of which 11 were
confirmed as the right company by comparing the platform's reported name.

## Goals / Non-Goals

**Goals:**

- Derive the orphan-company worklist from the catalogue, not from an external dataset.
- Propose candidate boards from the company name alone — no website, no search, no LLM.
- Make a seed-supplied employer name a gate, not a label, so a harvest cannot onboard the
  wrong tenant.
- Reuse `cmd/harvest-boards` as the only writer of `sources/*.yml`.

**Non-Goals:**

- Resolving company websites or searching the web for a company's board. That is what the
  July attempt did and where it died; if the 15% ceiling proves too low, it is a separate
  change.
- Providers whose board id is not a name: Workday (`tenant|dc|site`), gupy (numeric id),
  iCIMS, Oracle, Taleo, Cornerstone, PageUp, NeoGov. A company name cannot produce those.
- Any change to how boards are ingested, deduplicated, or enriched once onboarded.

## Decisions

### The seed is provider-agnostic; the provider loop lives in the run, not the tool

`harvest-orphans` writes one seed of `{board, company}` pairs and no provider. The operator
runs `harvest-boards <provider> orphans.seed.json` once per name-slug provider.

The alternative — have `harvest-orphans` emit `<provider>.seed.json` per platform — would
duplicate knowledge that already lives in `harvest-boards`: which providers exist, how a
board id is de-duplicated per provider (Workday folds case, Ashby does not), and which
boards the file already holds. Per-provider seeds would also be identical files under
different names, since a name-derived candidate is the same string for every platform.

The cost is honest: ~21k candidates × ~20 providers ≈ 250k probes per full run. This is a
run-once host tool and the probe fan-out is already bounded and polite.

### A prober reports "" when the platform publishes no name

The gate needs to tell "the platform published no employer name" from "the platform
published a name that disagrees". The tool used to encode the first case as "the returned
name equals the board id" — a slug echoed back. That inference is wrong for two probers:
`workdayProber` returned the tenant and `opencatsCompanyName` the host, neither of which
equals the board id, so both would have read as published names and armed the gate against a
token the employer never chose. Since `cmd/harvest-ats` and `cmd/harvest-role` already emit
seeds carrying an expected employer, a Workday run would have rejected every live board and
exited 0.

So the contract is explicit instead of inferred: a prober returns `""` when the platform
publishes no name. That is behaviour-preserving for the ~14 probers whose fallback token was
the board id — `chooseCompany` already treated those as nameless — and it fixes the two
where the token was derived. `orSlug` and the equality inference are both gone.

### The gate compares normalized names, and only when both sides exist

Rejecting a candidate needs both an expected name (from the seed) and a published name (from
the platform). When either is missing there is nothing to compare: seeds without a company
validate on live jobs alone, and platforms that publish no name take the seed's name as their
label. An expected name that normalizes to nothing (punctuation alone) states no expectation
either, and is treated as absent rather than rejecting the board.

This limits where the gate can fire at all, and the limit is wide: of the tool's ~30 probers
only greenhouse, workable, smartrecruiters, teamtailor, join, gupy, paycom, opencats and —
newly — recruitee and breezy obtain a name. The rest were audited against their live APIs;
Lever, Ashby, Personio, BambooHR, iCIMS, Traffit and the HTML-scraped platforms publish no
employer name anywhere in their public payloads, so for them a board is still accepted on
liveness alone and labelled from the seed. Extracting a name where one exists (recruitee's
`company_name`, breezy's `company.name`, both verified against live boards) is part of this
change; inventing one where it does not is not.

Normalization is deliberately mild: case-fold, strip legal-form suffixes
(`inc/llc/ltd/limited/gmbh/corp/co/bv/ab/oy/as`), collapse everything non-alphanumeric.
Measured on the sample this is what admits `Arch Capital Group Ltd.` against the platform's
`Arch Capital Group` while still rejecting the bamboohr tenant whose only posting is called
"Fake job". Substring matching was considered and rejected: it would admit any company whose
name contains a short common word.

A rejection is counted and logged apart from an unreachable candidate. Folding the two
together would hide a systematic mismatch — a probe that started returning a platform-wide
name instead of a tenant name — inside the normal noise of dead boards.

### Placement of the gate: `probeAll`, not `chooseCompany`

`chooseCompany` answers "what do we call this board"; the gate answers "do we keep it". They
are different questions and the second must be able to discard. Putting the decision in
`probeAll`, where the probe result and the seed entry are both in hand, keeps
`chooseCompany` a pure labelling function and leaves the rejection counter next to the
existing failure counter.

### Aggregator membership is a taxonomy question, not a capability one

`sources.All(c)` registers usajobs, reed and whatjobs only when their credentials are in the
environment — deliberate, so an unconfigured process does not list a provider it cannot
crawl. Deriving the exclusion set from that registry would therefore make a company held only
by Reed look ATS-covered whenever the tool runs without Reed's key, which is every run: the
tool needs `DATABASE_URL` and nothing else. The failure is silent and one-directional —
genuine orphans vanish from the worklist, exit code 0.

So `sources.AllAggregatorProviders()` answers the classification question independently of
credentials, and the crawl registry keeps answering the capability one. `cmd/reindex`'s
suppression pass asks the same classification question and now uses it too; on prod, where
the credentials are set, its behaviour is unchanged.

### The worklist query asks for absence of ATS, not presence of aggregator

A company qualifies on `NOT EXISTS (a non-aggregator open posting)`, evaluated against the
*full* aggregator set, while the candidate scan is narrowed to the requested aggregators.
Splitting the two sets matters: narrowing a run to himalayas must not make a remoteok
posting look like first-party ATS coverage. The July audit of this same distinction found
that using a partial aggregator list inflated its results roughly fourfold.

## Risks / Trade-offs

- **A name-derived slug resolves to an unrelated tenant.** → The corroboration gate is
  exactly this defence, and it is the change's primary deliverable rather than a side
  effect. It cannot cover everything: a platform that publishes no employer name gives the
  gate nothing to test, and that is most of them. The orphan run's yield is concentrated in
  Workable and SmartRecruiters, which both publish names, so the bulk of what this change
  onboards is gated — but a board harvested on Lever or BambooHR is still accepted on
  liveness alone and must be read as such in the PR diff.
- **A probe answers for a demo or template tenant.** → Live jobs plus a matching company
  name is a much narrower target than live jobs alone; the sampled bamboohr "Fake job"
  tenant was rejected on the name.
- **250k probes across ~20 providers is slow, and bamboohr is the slow pole** — dead
  subdomains hang to the client timeout. DNS pre-filtering was investigated and does not
  work: bamboohr, recruitee, breezy and personio all serve wildcard DNS, so every nonsense
  subdomain resolves. → Accept it; the tool is run-once, its fan-out is bounded, and
  providers can be run one at a time.
- **~1,200–1,500 new boards raise ingest time and enrichment queue depth proportionally.**
  → The operator was explicit that the queue should absorb it. Board files reach prod as
  bind-mounted data, so no image rebuild is involved and a bad batch is reverted by
  reverting the YAML.
- **The worklist scan is one long read over `jobs`.** Measured on the production catalogue
  (2026-08-01): a Nested Loop Anti Join driven by `jobs_source_id_open_idx` with
  `jobs_open_company_created_at_id_idx` on the inner side, **5.8s for 8,562 companies** — no
  sequential scan. The risk is a future planner flip to a hash anti-join, whose inner
  relation is the whole open table; the tool pins one connection with a 10-minute
  `statement_timeout` so that cannot become an unbounded read holding a snapshot open.
- **A URL-imported posting counts as ATS coverage.** `weblink` (a one-off user link import)
  is not an aggregator, so a company whose only non-aggregator row came from one is excluded
  even though no board of theirs is crawled. Conservative in the safe direction — a missed
  candidate, never a wrong board — and left as-is rather than special-cased.
- **A stripped name can shrink to a short generic token.** "Sky Co" yields the candidate
  `sky`. On the platforms that publish no employer name such a board is accepted on liveness
  alone, so the PR diff is the only backstop. Raising the minimum length would cost the real
  three-letter employers (ibm, sap), which is the worse trade.
- **A harvest run and a reindex must not collide.** → Onboarding is a normal deploy plus
  per-provider ingest; any manual reindex that follows must stop `freehire-reindexw.timer`
  first, per the standing rule.

## Migration Plan

1. Merge the tool and the gate; nothing changes in production until a harvest is run.
2. Run `harvest-orphans` against the eight remote-jobs aggregators to produce the seed.
3. Run `harvest-boards <provider> orphans.seed.json` for each name-slug provider; review the
   combined YAML diff as one PR.
4. Deploy with `--no-build` (board files are bind-mounted data) and ingest each changed
   provider.

Rollback is reverting the YAML additions: a board that is no longer listed is no longer
crawled, and the postings it already produced are handled by the normal lifecycle.

## Open Questions

None. Scope, the acceptance gate, name-match strictness, and rollout pace were settled
before this document was written.
