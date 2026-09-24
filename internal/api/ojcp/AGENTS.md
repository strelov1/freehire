# OJCP projection conventions

## Scope
Projecting this catalogue into the wire shapes of the **Open Job Context Protocol** (OJCP,
draft v0.1) — the standard AI agents read job data through. This package is the projection
and the manifest; the two transports that publish it live elsewhere:

| Where | What |
|---|---|
| `internal/api/ojcp` (here) | the projection, the response envelopes, the manifest |
| `internal/api/handler/ojcp.go` | the REST transport, and the methods both transports call |
| `internal/api/ojcpmcp` | the MCP transport, an adapter over those same methods |

## Always true

- **Pure projection.** Nothing here reads the database, opens a connection, calls
  `time.Now()`, or knows Fiber exists. Every function takes an already-loaded domain value
  and returns a struct. That is what lets both transports share one implementation instead
  of two that drift, and what lets every projection be validated against OJCP's own
  published JSON Schemas with no fixture beyond a domain value.

- **The vendored schemas are the oracle, and they prove less than they appear to.** See
  [testdata/schemas/README.md](testdata/schemas/README.md) — it records their provenance and
  the two structural limits (no `additionalProperties: false`, and a search envelope that
  inlines a laxer job shape instead of referencing the posting schema). A green schema check
  is not a green projection; presence of the fields we mean to emit needs its own assertion.

- **A field the posting does not state is OMITTED, never emitted empty.** An empty
  `datePosted` is both schema-invalid and — worse — reads to an agent as a date we claim to
  know. A zeroed `baseSalary` reads as a job that pays nothing. An empty `jobLocation` says
  "we know where this is" and then says nothing.

- **A closed enum omits rather than guesses; an open one passes ours through.**
  `remote_policy` and `baseSalary.unitText` are closed in the schema, so an untranslatable
  value drops the field (or, for salary, the whole block — a daily rate read as an annual
  one is a wrong answer, not a partial one). `experienceLevel` is an open string whose own
  description invites additional values, so `experienceLevel` translates only the two levels
  whose meaning is identical in both vocabularies (`middle`, `c_level`) and the remaining SIX
  go out verbatim — `intern`, `junior`, `senior`, `lead`, `staff`, `principal`. Collapsing
  `staff` and `principal` into `senior` would make an agent's search for one return the
  other; collapsing `intern` into `entry` would answer a junior search with internships.

- **The vocabulary maps are guarded against the vocabularies they mirror.**
  `vocabulary_test.go` walks `vocab.WorkModeValues`, `vocab.SalaryPeriodValues` and
  `ghost.CriterionCodes` and asserts each value has a DECIDED answer — translated, or
  deliberately not. All three are OUTBOUND maps, and in an outbound map a deliberate gap is
  recorded as an EMPTY VALUE, never as a missing key: the two behave identically at runtime,
  and only the empty one tells a reader it was decided. A test enumerating the same list the
  map does would prove only that somebody typed it twice.

- **`seniorityFromStandard` is the exception to the empty-value rule, because it is
  INBOUND.** An empty value there does NOT behave like a missing key. `QueryValues` branches
  on the lookup's `known`, so an empty value takes the found path and applies
  `seniority=""` — an empty filter, silently widening; a missing key reports
  `filters.experience_level` unsupported, which is the honest answer. `director` is
  therefore absent rather than empty, and that is deliberate.

  It is also guarded as a ROUND TRIP rather than a lookup, because its two halves live in
  different files: `seniorityFor` (`jobposting.go`) publishes our value verbatim where the
  standard has no word for it, and `seniorityFromStandard` (`searchinput.go`) reads it back
  as a filter. They drifted — we published `intern`, `junior`, `staff` and `principal` while
  the filter knew only the standard's five, so an agent that read one off our own posting and
  sent it back had its filter dropped and its answer widened to the whole catalogue. A lookup
  test on either map alone would have stayed green. The round trip also rejects an empty
  value outright, which is what holds the exception above in place.

- **`skills_required` comes from the posting's stated REQUIREMENTS, never from the skills
  facet.** The facet resolves every skill named anywhere in the description, "nice to have"
  blocks included; publishing it as required tells an agent a candidate without Kubernetes
  does not qualify for a role that called Kubernetes a plus. `skilltag.PreferredFromText`
  runs over each priority block — it returns the curated SPELLING (an agent gets
  `PostgreSQL`, not our internal `postgresql` key) and judges the whole block at once, which
  is what lets an ambiguous word like "Go" through where the text also names a technology
  outright. A posting with no stated requirements says nothing for either field.

- **`official_job_url` is the employer's own page, or nothing.** It is only filled for a
  direct ATS or a company careers page (resolved once through `sources.ProviderKind`), and
  the `utm_source` jobview stamps on every served URL is stripped with
  `outboundurl.Untag`. The schema defines this field as a domain-verification anchor that
  agents deduplicate against, and an aggregator's stored URL is a tracking redirect on its
  own domain — `adzuna.com/land/ad/<id>` is not the employer's page. Such a posting keeps
  its `url`, the field whose description permits aggregators. **A clicked link keeps the
  tag**: our own page and `apply_paths[].url` both carry it.

- **`supports_agent_submission` is derived from this DEPLOYMENT, and is pessimistic.** It is
  handed in through `Projector.Submittable` (the handler reads
  `atsapply.SubmittableProviders`), and additionally goes false when the form requires a
  file that is not the résumé — such an attempt parks before the provider is consulted. The
  résumé test here is stricter than atsapply's own, deliberately: a false negative costs an
  agent an opportunity, a false positive costs a candidate a daily tailoring turn on an
  attempt that cannot finish.

- **`required_fields` goes through `Form.ForDisplay`, not a filter written here.** That
  reader already drops the platform's hidden controls (a real Greenhouse form requires
  "Longitude"), the mid-form text blocks, the consent boilerplate and the equal-opportunity
  survey; flattens labels authored as HTML; and names a question once where the platform
  spread it over several controls. A second copy of those rules would be a second thing to
  keep current.

- **The posting-reality verdict travels in `agent_notes`,** because OJCP has no field for
  it. The wording keeps the interface's doctrine — facts about a POSTING, never a claim
  about an employer — and a test forbids the accusatory vocabulary outright, since an agent
  relays this sentence to a candidate and possibly to the employer. Criteria are rendered as
  sentences, never as their internal codes.

## Known gaps in the standard

Both are candidates for the same RFC, and both are the reason to hold a seat rather than ask
for one:

- **No field for posting reality.** Hence `agent_notes`, above.
- **Multi-country reach is not expressible.** `jobLocation` is ONE schema.org `Place`, and a
  posting open in three countries does not have one. An ambiguous facet is omitted rather
  than resolved by taking the first value — the list order carries no precedence, and an
  agent filtering on an arbitrary choice would drop the posting everywhere else the employer
  accepts.

## What the projection cannot do for itself

- **`agent_notes` needs a verdict ATTACHED.** `Ghost` is not intrinsic to a `jobview.Job` —
  it is time-dependent and never stored, so every surface that wants it attaches its own
  (`jobs.go`, `search.go`, `me_tracking.go`, and now the OJCP detail handler). A projection
  that merely READS `j.Ghost` publishes nothing, for every posting, forever. That is exactly
  what this surface did until a review walked the call graph instead of the tests, which
  were green because they set the field by hand.
  The SEARCH tool deliberately carries no verdict: two more queries per page for a field an
  agent must open the posting to act on, and `get_job_detail` is one call away.

- **Visibility is the handler's job.** `GetJobBySlug` carries NO predicate — it is the read
  a private job's own creator uses, and the detail page relies on it to serve a closed
  posting. Neither is right here: an agent enumerates, caches and republishes what it is
  handed. `publishedToAgents` refuses private, closed and duplicate-suppressed rows, and
  answers NOT FOUND rather than forbidden — whether a private posting exists under some slug
  is not an anonymous caller's business.

## Two traps

- **`addressRegion` is a state or province.** Our `Regions` facet holds macro-regions
  ("europe", "global"). Writing one into the other reads to an agent as a province named
  Europe.
- **Country codes are stored lowercase** by `jobview.normalizeSet`, while every country
  field in this standard is ISO 3166-1 alpha-2 — uppercase. This has bitten twice already
  (a posting's `addressCountry` and an employer's `hq_location.country`).

## A declared schema constant that is never used is a switched-off check

The error envelope shipped in a shape the standard does not describe at all — the schema
wants a FLAT `error_code` from a closed enum, and this package sent a nested
`error: {code, message}` with codes of its own invention. Nothing caught it because
`schemaErrorResponse` was declared beside the others and never passed to a single check.

From the inside that looks exactly like a working check. When adding a response shape, add
the validation in the same commit, and make sure it can FAIL — the rate-limit refusal turned
out to require `retry_after_seconds`, which only surfaced once the oracle was finally
pointed at that envelope.

## A guarantee written in a comment is not a guarantee

Two of these shipped in one commit:

- An error-status table argued that a missing entry *"would serve status 0, which Fiber
  rejects"*. **Measured: Fiber serves status 0 as `200 OK`** with the error body, so an agent
  reads success. The protection had to become a `switch` with a real `default`.
- `ojcpmcp.toolError` returned plain Go errors while its own comment said the SDK would turn
  them into JSON-RPC errors. It does that only for a `*jsonrpc.Error`; anything else becomes a
  tool result with the message as text, so **no OJCP envelope ever reached an agent over MCP**
  — and that package had no tests at all.

When a comment states what a library does, measure it. Both took under a minute to check and
both were wrong.

## Check with the client that will actually use it

The manifest was verified by unit tests, two reviews, OJCP's conformance suite and a `curl`
after every deploy. All of them saw `200 OK`. **None of them was a browser**, and the
same-origin policy is enforced only by browsers — so the registry's own submission form
could not read the document at all, for want of an `Access-Control-Allow-Origin` header
nobody had thought to serve.

The lesson is not "remember CORS". It is that a check is blind to whatever its client does
not do. When adding a surface, ask who calls it — a browser, an MCP client, a plain HTTP
client, a crawler — and exercise it as at least one of each. The four checks above were four
of the same kind of client wearing different hats.

## Testing

Build fixtures with `jobview.FromRow(db.Job{...})`, never as a `jobview.Job` literal. The
literal skips `outboundurl.Tag` and `normalizeSet`, so it carries values no read path
produces — which is exactly how a utm-tagged `official_job_url` and a lowercase
`addressCountry` both passed green until a review caught them.
