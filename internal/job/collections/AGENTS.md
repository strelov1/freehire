# Curated company collections conventions

## Scope
The code-owned registry of curated company tags: editorial themes (Big Tech, Unicorn),
credentials from authoritative public registers (UK/NL/US visa sponsors), and backers (YC,
Techstars, a16z). This package is the source of truth for which tags exist and how members
are matched; `cmd/import-collections` writes `companies.collections`, and the search facet
(`jobs.collections`) serves it.

## Always true
- **The registry is code.** `All` (collections.go:167-286 — 16 slugs, in display order) is
  the fixed set; adding a tag is one entry with exactly one membership source: a static
  `Slugs` hand list or a `Dataset`. `Kind` decides how the tag renders (editorial chip /
  credential with issuing body and snapshot date / backer brand mark) and its zero value is
  deliberately invalid so a forgotten one fails the registry test (collections.go:99-110).
- **A partial dataset read reconciles the tag OFF every company it failed to reach.** A
  `Dataset.Records` source must read completely and error otherwise — a partial read is
  indistinguishable from a shrunken source (collections.go:34-59). `Dataset.Valid` enforces
  exactly one of URL / embedded Data / ResolveURL / Records (collections.go:61-79).
- **Both kinds match on `normalize.CompanySlug`** — `RegisterSlug` is now a thin caller of it
  (register.go). Editorial matching used `normalize.Slug` until the catalogue began stripping
  corporate forms; a dataset naming "Acme Robotics Limited" then asked for a slug ingest can no
  longer produce, matched nothing, and said nothing. The rule has to be ONE rule because
  `Members` looks its output up in a map keyed by the catalogue's own company slug — see
  [docs/agents/company-identity.md](../../../docs/agents/company-identity.md). What still separates
  a credential is `DropAmbiguous` and the gates, not the spelling.
- **Hand lists are guarded, because they silently depend on that rule.** Every entry must be a
  fixed point of `CompanySlug`; the test that pins this found three `eastern_roots.txt` entries
  already tagging the wrong company (`epam-systems-pte-ltd`, 19 open jobs, instead of
  `epam-systems`, 1,172). An entry the rule would never produce matches nothing, and a
  collection that matches nothing reports no error. `TestHandListSlugsAreCompanySlugStable`
  covers the static `Slugs` lists AND both embedded files — a new membership file must be
  added to its map, or it is unguarded.
- **A hand list is a list we wrote, whichever way it is delivered** (`Collection.HandList`,
  collections.go:154-162). Both the static `Slugs` lists and the embedded `.txt` files are ours
  to fix, so their unmatched entries are logged BY NAME; a fetched third-party dataset has
  thousands and only its count is. The import worker drew this line as `Slugs != nil` until
  2026-08-31, which silenced the two largest hand lists we have: a typo in `eastern_roots.txt`
  or `indian_roots.txt` matched nothing and said nothing, which is the exact failure the
  fixed-point test exists to catch on the other axis.
- **`RequireCountry`'s asymmetry is the point** (register.go:54-77): a multi-token name
  ("Acme Robotics") needs only open jobs in the register's country; a single-token name
  ("Apple", "Spark") additionally requires HQ there, or a multinational with a local office
  inherits a licence from a same-named local business. Unknown HQ is not a match — absence
  of evidence is not evidence.
- **`countryAliases` exists because a retired one-time importer left `hq_country` rows
  its upstream's writer never normalized** (register.go:79-94): `cmd/import-yc`, the
  column's live writer, parses through `location.Parse`, but those legacy rows still carry
  the upstream's spelled-out value verbatim. The failure mode is silent — a company simply
  never earns a credential it qualifies for. Whole-value comparison only: "New Great
  Britain Holdings" is a company name, not a country.
- **`DropAmbiguous` drops register rows whose name identifies more than one organisation**
  (register.go:143-174), using `Dataset.IdentityKey` (UK town, NL kvk, US tin4). Rows
  sharing a name AND an identity are one body listed once per route — they must survive for
  `RequireRoute`. With an empty IdentityKey the guard is permissive on purpose: firing it
  on a register that cannot disambiguate would delete its every duplicate, and the
  geography gates are the real defence there.
- **A company is tagged when ANY of its records passes the gate** (collections.go:469-496):
  a register lists an organisation once per route it holds, so a work-route row must win
  even when a temporary-route row sorts first.
- **`Reconcile` is given live + retired slugs** so a removed or renamed tag is stripped
  everywhere on the next run (collections.go:537-562; `RetiredSlugs` collections.go:447-452;
  wired at cmd/import-collections/main.go:358). Tags the registry does not manage are
  preserved untouched.

## How it works
`cmd/import-collections` resolves each collection's dataset, matches records to the
catalogue (`Members`), and writes `companies.collections` via `SetCompanyCollections`
(main.go:128). Dataset fetches egress through `SOURCES_PROXY_URL` when set — always falling
back to direct (main.go:196-228) — and a per-collection `<SLUG>_DATASET_URL` override
(e.g. `YC_DATASET_URL`) wins over the pinned URL (main.go:410-425). `cmd/harvest-ats`
re-reads the same datasets to mine new companies for board discovery
(cmd/harvest-ats/main.go:107). Unmatched hand-list entries are logged by name, capped at
`MaxLoggedUnmatched` (50) so a mis-edit cannot flood a run — which means a membership file
should not carry entries it knows will not match. `indian_roots.txt` was written with a
90-slug tail of marquee companies we do not ingest yet, on the theory that the tag would
land the day their board arrived; a run then reports 90 intentional misses and a real typo
sits somewhere among them. Add a company when its board lands, not before.
