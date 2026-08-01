## Context

The board harvest is already split into three roles: something supplies a worklist of
companies, `cmd/harvest-ats resolve` follows each company's website to its ATS board, and
`cmd/harvest-boards` probes every candidate slug against the provider's official API and
commits only what answered. Only the first role is weak. Its worklists are the curated
collection datasets and a world-universities directory — static, undated, and silent about
whether anyone at those companies is hiring.

LinkedIn's public `jobs-guest` endpoints answer that directly, need no credentials, and were
verified during design to serve both the search listing and each posting's structured
metadata under this project's own User-Agent. Two facts were confirmed present on a live
sample:

- the posting's JSON-LD `hiringOrganization.sameAs` leads to the employer's public profile,
  whose own JSON-LD `Organization.sameAs` is the employer's website — resolved for 20 of 20
  companies sampled;
- the posting's JSON-LD `identifier.value` is the posting's id **in the employer's own ATS**
  (`teamtailor-8094978`, `4698693006`, `JR37734`, …) — present on roughly half the sample.

Three things LinkedIn does *not* give, all checked directly: the offsite apply URL (hidden
behind a sign-in modal), the legacy `applyUrl` element (removed from the markup), and ATS
links inside posting descriptions (stripped — 0 of 20). So the board must be *derived*, not
read, and the value of `identifier.value` is that it makes a derived board provable.

## Goals / Non-Goals

**Goals:**

- Feed the existing harvest a worklist of companies that are demonstrably hiring now, for
  any keyword and market the operator lists.
- Widen `harvest-ats` detection to the full board recognizer, including platforms that host
  the careers site themselves.
- Make a JS-only careers page a solvable case instead of a skipped one, by proposing
  candidate slugs that `harvest-boards` confirms against an exact posting id.
- Keep every LinkedIn-derived fact transient: only boards this project validated against a
  provider's own API reach `sources/*.yml`.

**Non-Goals:**

- Ingesting LinkedIn postings as a catalogue source. That is a far denser request shape, it
  duplicates postings we already take from the boards themselves, and it would put LinkedIn
  content in our database rather than a worklist in `/tmp`.
- Any persistent state: no table, no cursor, no cron. These are run-once host tools, as the
  rest of the `harvest-*` family is.
- Browser impersonation, headless rendering, or proxying. If a page needs JavaScript, the
  identifier path handles it; if LinkedIn stops serving us, the run fails loudly.

## Decisions

**One thin new binary, two surgical extensions — not one self-contained tool.**
`cmd/harvest-linkedin` only discovers companies; it never talks to an ATS. The confirmation
step lives in `harvest-boards` because it is an improvement to *board validation*, not a
property of LinkedIn — `harvest-role`, which likewise knows a posting's id, can supply the
same field tomorrow. The rejected alternative was a single binary that probes provider APIs
itself: it would put second copies of the Greenhouse and Lever probe logic beside the ~15
probers that already exist.

**Slug candidates are derived offline and confirmed remotely.** Deriving a slug from a
domain, profile slug or company name is guesswork, and would be unacceptable on its own. It
becomes acceptable because the seed carries the posting id that decides it: a candidate is
kept only if the platform reports a live posting with exactly that id. A wrong slug that
happens to be a real board belonging to someone else is rejected on evidence. This also
means derivation needs no network, which keeps it a pure, cheaply-tested function.

The identifier's shape narrows the provider set: `teamtailor-<digits>` → teamtailor, ten
digits → greenhouse, a UUID → lever and ashby. Shapes that narrow nothing (`JR37734`,
`R00314608_en`) yield no candidates rather than a fan-out across every provider.

**Confirmation rides on an optional interface, not on the `prober` contract.** `prober.go`
carries ~15 implementations behind one interface; widening that interface would touch all of
them for the benefit of six. Instead a separate, optional interface reports the ids of a
board's live postings, and is consulted only when a seed entry carries an expected id.

It is implemented only where a single request yields the board's *complete* live list —
greenhouse, lever, ashby, recruitee — because the check reads a missing id as a wrong board.
SmartRecruiters is probed with `limit=1` and Teamtailor reads only its first page, so for
them an absent id would mean "not on the page I looked at", and they are left inert rather
than made to reject boards on partial evidence. Everything else is untouched, and an expected id on a
provider that cannot check it is inert rather than fatal.

**`harvest-ats` gains self-hosted detection, and nothing else.** The plan had been to detect
through `internal/boardresolve`, on the belief that `atsdetect.Detect` recognised only three
providers. Implementation disproved it: `atsdetect.Detect` falls back to scanning every URL
on the page through `FromURL`, which *itself* calls `atsboard.Recognize` and then adds five
harvest-only shapes on top. Routing the harvest through `boardresolve` would therefore have
**narrowed** its coverage by those five, in exchange for one step it genuinely lacked.

So the change is that one step: `atsdetect.DetectSelfHosted`, tried only after the URL scan
comes up empty. Radancy, Phenom, Jibe and Teamtailor tenants serve from the employer's own
domain and link to no ATS host, so no URL scan can ever see them; the vendor's bundle in the
markup is the only tell, and the careers host is the board. Order matters — a careers site
that merely embeds another ATS must resolve to that board, not to its own host — which is why
the fallback is last rather than first. `boardresolve` stays untouched.

**Company de-duplication happens before detail fetches.** The search card already names the
employer and links its profile, so the catalogue filter and the collapse of many postings to
one employer both run on the listing alone. On the design sample, 48 postings covered 35
employers — about a third of the detail traffic never needs to be sent.

**An all-empty run exits non-zero.** A blocked or re-templated LinkedIn returns exactly what
an empty market returns. The project has been burned by this shape before (`ingested=0
failed=0` reading as a healthy board), so silence is treated as failure at the run level,
while a single empty query is only a warning.

**Requests go through `sources.Client` plus `internal/sources/pacer.go`.** Retries, `Retry-
After` handling, body caps and SSRF protection already live there, and the client's
User-Agent is the project's own. Verified during design: LinkedIn serves both endpoints under
it, so no browser impersonation is needed and none will be added.

## Risks / Trade-offs

- **LinkedIn changes its markup or starts refusing us** → The run fails loudly instead of
  quietly: an all-empty run exits non-zero. Parsing follows the sample-per-posting shape, so
  one changed card costs one posting, not the run. Nothing downstream depends on LinkedIn
  being available — it is a worklist source, and the other worklists keep working.
- **Terms-of-service exposure from automated access** → Volume is deliberately small, hand-
  run, rate-limited and identified as this project. No LinkedIn posting content is stored or
  republished; what enters the repository is boards confirmed against each provider's own
  public API, which is the standing rule for `harvest-boards` seeds.
- **A derived slug is a real board owned by someone else** → Rejected by the expected-id
  check, and reported as an id mismatch so it is visible rather than silently absent.
- **Half the sampled postings carried no ATS-native identifier** → Those companies still flow
  through the ordinary careers-page path; the identifier only adds a second route, it does
  not gate the first.
- **Confirmation costs extra probes (up to three slugs × two providers per unresolved
  company)** → Bounded by construction, and only spent on companies that produced no board
  at all — which today produce nothing.
- **Deriving the provider from an id's shape is heuristic** → It never decides anything by
  itself; it only chooses which boards to ask. A wrong guess costs one API call and is
  rejected.

## Migration Plan

Nothing to deploy and nothing to roll back: three host tools and one query file, none of
which run in production or touch the database. The `harvest-ats` detection swap and the
`harvest-boards` seed field are backward compatible — seeds without an expected id validate
exactly as before. First use is a manual run whose diff to `sources/*.yml` is reviewed like
any other harvest PR.

## Open Questions

- What share of discovered companies is genuinely new is unknown until the first run; the
  design sample skewed toward large consultancies we almost certainly already have. The run's
  own counters answer this, and the query worklist is the dial to adjust.
- Which markets and keywords to seed the worklist with is deliberately left to the first
  run's results rather than guessed now.
