## Context

`sources.All(c HTTPClient) map[string]Source` builds the adapter registry. It is called two
ways, and the two callers want different things:

- **Crawl registry** — `All(sources.NewClient())` in `cmd/ingest`, `cmd/liveness`,
  `cmd/resolve-url` and `internal/handler`. These fetch, so a provider without its credential
  must be absent: config validation then fails fast on a board file naming it, instead of
  starting crawls that 410 or 429 per board and cool the boards.
- **Taxonomy registry** — `All(nil)` in `cmd/reindex`, `cmd/ghost-crosscheck`,
  `internal/handler/status.go`, `sources.FilterableProviders()` (and through it
  `cmd/gen-contracts`). These never touch transport; they read `Provider()` and the marker
  interfaces `aggregator()` / `boardless()` / `sweepGrace()` to answer *what kind of source is
  this*, which is a compile-time fact about the adapter type.

`registry.go` already recognises the split for four adapters. `taleo` needs a cookie-persisting
client and `meta` / `bayt` / `gulftalent` need the Chrome-fingerprint transport, so all four
register with a `nil` client on the `c == nil` path and with their special transport otherwise,
with the reasoning written in place: "Provider()/boardless() never touch it". The three keyed
adapters were written before that pattern and gate registration itself, not the transport.

Consequences observed in the repo:

- `cmd/reindex/main.go:420-425` — `AggregatorProviders(All(nil))` drops `whatjobs`, a CPC
  reseller of first-party ATS postings, on any keyless reindex host. Production serves 6,298
  `whatjobs` rows, none of them eligible for ATS-twin suppression while this holds.
- `web/src/lib/generated/contracts.ts:1012` — `SOURCE_VALUES` has no `usajobs`, `reed` or
  `whatjobs`; the file was generated without the credentials in the environment. Harmless
  today — the source facet is `dynamic: true` and reads the live distribution, and nothing in
  `web/src` imports `SOURCE_VALUES` or the `Source` type — but the constant is checked in, so it
  reads as authoritative to the next person.
- `cmd/ghost-crosscheck/main.go:85-86` — same registry, same leak, uncited.
- `internal/handler/status.go:123,134` — `ProviderKind` answers `KindOther` for all three.
- `internal/pipeline/AGENTS.md:11` says one adapter is keyed; `internal/sources/AGENTS.md:12`
  says three. Both describe the conflated rule.

## Goals / Non-Goals

**Goals:**

- `All(nil)` is total over the provider taxonomy: every adapter the binary knows about is
  present, with correct markers, regardless of the environment.
- `All(client)` keeps today's credential gate byte-for-byte, so ingest's fail-fast validation
  and the "never start a crawl that cannot authenticate" rule are untouched.
- The rule is stated once, in code, where the next keyed adapter will read it.

**Non-Goals:**

- Moving the credential check to `Fetch` time. That would let `cmd/ingest` start a crawl that
  fails per board and cools every board of the provider — strictly worse than failing at
  config validation.
- Extracting the taxonomy into a separate list or a generated table. There is no second
  producer of the markers, and a parallel list is a second thing to keep in step.
- Any change to the crawl behaviour, board files, or the ingest pipeline.

## Decisions

**Register the keyed three on the `c == nil` path with an empty credential, mirroring
taleo/meta.** The credential is a struct field on a plain value type
(`usajobs{http, apiKey}`, `reed{http, apiKey}`, `whatjobs{http, publisherID}`) — no
constructor validates it, so `NewReed(nil, "")` is as safe to build as `NewTaleo(nil)` is
today. Nothing on the taxonomy path can reach the field: `Provider()` and the three marker
methods have value receivers that ignore both `http` and the credential.

*Alternative considered — a `taxonomy()` registry built from a separate adapter list.* It
decouples the two questions more explicitly, but it introduces a list that must be updated
alongside every `reg(...)` entry, with nothing to catch a miss. The registry already has one
authoritative construction site; the fix should keep it that way.

*Alternative considered — keep registration gated and teach each taxonomy consumer to union in
the missing three.* That spreads the same knowledge across four call sites, which is the
defect restated.

**Name the second mode: `sources.Taxonomy()`.** `All(nil)` and `All(client)` mean two different
things, and spelling both as one function leaves the distinction to a nil argument the reader
has to know about. `Taxonomy()` is a three-line wrapper over `All(nil)` that gives the rule
somewhere to live — "carries no transport, so nothing here may Fetch; crawlers call `All`" — and
makes every classifying call site say what it is doing: `cmd/reindex`, `cmd/ghost-crosscheck`,
`internal/handler/status.go` and `FilterableProviders`.

*Alternative considered — leave `All(nil)` as the convention.* It is the smaller diff and the
convention already carries `taleo`/`meta`/`bayt`/`gulftalent`. Rejected because the finding is
precisely that two questions share one entry point; fixing the registration without naming the
split leaves the next reader in the same position. `Taxonomy` is a wrapper, not a second
registry, so there is nothing to keep in step.

**An adapter built on the taxonomy path stays uncrawlable by construction, not by a check.**
With `c == nil` there is no transport, so a `Fetch` would fail immediately — exactly the
existing situation for `taleo`, `meta`, `bayt` and `gulftalent`. No consumer of `All(nil)`
calls `Fetch`, and adding a defensive "empty credential refuses to fetch" branch would be a
guard against a caller that does not exist. Recorded here as the seam if one ever appears.

**Regenerate contracts so the checked-in file stops varying with the operator.**
`SOURCE_VALUES` is generated from `FilterableProviders()` and is currently the visible symptom
of the conflation, even though no runtime code reads it. Regenerating is part of the fix
because otherwise the next `make gen-contracts` on a configured machine produces an unrelated
three-line diff in someone else's PR.

**The two label overrides are adjacent work, and named as such.** `SOURCE_LABELS` is a
`Record<string, string>` with a title-case fallback, so nothing forces an entry — but the live,
distribution-driven facet renders `Usajobs` and `Whatjobs` today, which is the same
one-label-per-facet-code rule S1 settled. Two lines, in the same file the change already
touches; the alternative is a separate PR for two strings.

## Risks / Trade-offs

- **A reader may take "registered" to mean "crawlable".** → The doc comment names the two
  registries explicitly, and the adapter tests assert both halves: present in `All(nil)`,
  absent from `All(client)` without the credential.
- **`/api/v1/status` changes a `kind` value for three providers** (`other` →
  `aggregator`). → That is the fix, not a regression: the field is documentation of the
  adapter's kind and was wrong. No shape change, no client parses `kind` for control flow.
- **`SOURCE_VALUES` gains three options, so a consumer could offer a provider a given
  environment does not crawl.** → No consumer exists yet, and when one arrives the facet should
  describe what is *in the catalogue* — a property of the data, not of the local environment.
  Production already serves 8,214 `usajobs`, 8,135 `reed` and 6,298 `whatjobs` postings.
- **`cmd/ingest` regression would be silent if the gate were dropped from the client path by
  accident.** → A test asserts `All(client)` omits each of the three when its variable is
  unset, so the gate cannot be removed without a red test.

## Migration Plan

No migration. Deploy order is irrelevant: the change is a pure in-process registry fix plus a
regenerated frontend constant. Rollback is the revert.

The next `cmd/reindex` run after deploy re-marks `whatjobs` postings that duplicate a
first-party ATS posting — a correction of existing rows through the already-scheduled path, not
a backfill to run by hand.
