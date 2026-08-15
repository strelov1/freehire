# Curated company collections conventions

## Scope
The code-owned registry of curated company tags: editorial themes (Big Tech, Unicorn),
credentials from authoritative public registers (UK/NL/US visa sponsors), and backers (YC,
Techstars, a16z). This package is the source of truth for which tags exist and how members
are matched; `cmd/import-collections` writes `companies.collections`, and the search facet
(`jobs.collections`) serves it.

## Always true
- **The registry is code.** `All` (collections.go:157-269 — 15 slugs, in display order) is
  the fixed set; adding a tag is one entry with exactly one membership source: a static
  `Slugs` hand list or a `Dataset`. `Kind` decides how the tag renders (editorial chip /
  credential with issuing body and snapshot date / backer brand mark) and its zero value is
  deliberately invalid so a forgotten one fails the registry test (collections.go:99-115).
- **A partial dataset read reconciles the tag OFF every company it failed to reach.** A
  `Dataset.Records` source must read completely and error otherwise — a partial read is
  indistinguishable from a shrunken source (collections.go:46-52). `Dataset.Valid` enforces
  exactly one of URL / embedded Data / ResolveURL / Records (collections.go:64-79).
- **Credentials match on `RegisterSlug`, not `normalize.Slug`** (register.go:39-49): it
  strips one trailing legal form ("ACME ROBOTICS LIMITED" → `acme-robotics`), without which
  an exact match against a register finds almost nothing. Only the LAST token is considered
  ("Limited Brands" keeps its name); "co" is a deliberate omission — it collides with
  ordinary words inside genuine names. Editorial collections match on `normalize.Slug`
  unchanged — a changed rule would silently rewrite their membership.
- **`RequireCountry`'s asymmetry is the point** (register.go:78-101): a multi-token name
  ("Acme Robotics") needs only open jobs in the register's country; a single-token name
  ("Apple", "Spark") additionally requires HQ there, or a multinational with a local office
  inherits a licence from a same-named local business. Unknown HQ is not a match — absence
  of evidence is not evidence.
- **`countryAliases` exists because a retired one-time importer left `hq_country` rows
  its upstream's writer never normalized** (register.go:103-117): `cmd/import-yc`, the
  column's live writer, parses through `location.Parse`, but those legacy rows still carry
  the upstream's spelled-out value verbatim. The failure mode is silent — a company simply
  never earns a credential it qualifies for. Whole-value comparison only: "New Great
  Britain Holdings" is a company name, not a country.
- **`DropAmbiguous` drops register rows whose name identifies more than one organisation**
  (register.go:180-201), using `Dataset.IdentityKey` (UK town, NL kvk, US tin4). Rows
  sharing a name AND an identity are one body listed once per route — they must survive for
  `RequireRoute`. With an empty IdentityKey the guard is permissive on purpose: firing it
  on a register that cannot disambiguate would delete its every duplicate, and the
  geography gates are the real defence there.
- **A company is tagged when ANY of its records passes the gate** (collections.go:457-497):
  a register lists an organisation once per route it holds, so a work-route row must win
  even when a temporary-route row sorts first.
- **`Reconcile` is given live + retired slugs** so a removed or renamed tag is stripped
  everywhere on the next run (collections.go:512-532; `RetiredSlugs` collections.go:421-426;
  wired at cmd/import-collections/main.go:347). Tags the registry does not manage are
  preserved untouched.

## How it works
`cmd/import-collections` resolves each collection's dataset, matches records to the
catalogue (`Members`), and writes `companies.collections` via `SetCompanyCollections`
(main.go:117). Dataset fetches egress through `SOURCES_PROXY_URL` when set — always falling
back to direct (main.go:184-214) — and a per-collection `<SLUG>_DATASET_URL` override
(e.g. `YC_DATASET_URL`) wins over the pinned URL (main.go:400-406). `cmd/harvest-ats`
re-reads the same datasets to mine new companies for board discovery
(cmd/harvest-ats/main.go:107). Unmatched hand-list entries are logged, capped at
`MaxLoggedUnmatched` (50) so a mis-edit cannot flood a run.
