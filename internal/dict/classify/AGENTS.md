# internal/dict/classify — Seniority & Category Tagging

Deterministic seniority/category tagging from job title, feeding enrichment facets.

## Design

- Parses the **job title** at ingest into canonical `jobs.seniority`/`jobs.category` columns.
- Values from `vocab.SeniorityValues`/`vocab.CategoryValues` — EN+RU aliases, whole-word matched. Russian forms listed as full surface forms (not stems) since matcher requires word boundaries. **Never guesses**.
- Same alias→canonical dictionary design as `internal/dict/location` and `internal/dict/skilltag`.

## Grade-blind phrases

Some role names **contain** a grade word without stating a grade: "Member of Technical Staff" is a generic IC title, "Lead Generation" is a marketing function, "Middle East" is a region, "Mid-training" is a model-training stage. `gradeBlindPhrases` is cut from the title (each occurrence replaced by a space) **before** the seniority match, so only the remaining words can state a grade.

Ordering alone could not fix this: `staff` outranks `senior`, so "Senior Member of Technical Staff" resolved to `staff`. The mask leaves honest grades untouched — "Senior Staff Engineer" is still `staff`.

The list holds only phrases that shadow a `seniorityTable` alias; the category match reads the untouched title.

## UK early-careers vocabulary

"Graduate" is `junior` and the placement phrases ("Industrial Placement", "Year in Industry", "Summer Placement", "Vacation Scheme", ...) are `intern`. `graduate` is the LAST entry of the table — the weakest evidence it holds — so "Graduate Summer Internship" stays an internship and any explicit grade outranks it. Bare "placement" and bare "undergraduate" are deliberately absent: on prod the first names recruiters and the second mostly names university faculty and admissions titles. The staff desks that merely contain "graduate" or "placement" ("Graduate Admissions", "Graduate Recruiting Manager", "Director of Graduate Programs", "Work Placement Officer") are cut before the match by `earlyCareersDeskPatterns` — a shape (the word next to the desk it names), not a list of titles.

## The two design crafts

`design` means product/visual/experience design. Engineering draughting —
mechanical, electrical, civil, and the architectural/BIM family — is the separate
`engineering_design` category, a `vocab.NonTechCategories` member (surfaced as a
facet, off the LLM and embedding budgets). Silicon design is neither: it resolves to
`hardware`, where the rest of that team already sits.

Three groups sit **before** the bare `designer`/`design` entries, because the word
`design` alone would otherwise claim every "… Design Engineer": the titles that name
another craft (`network design engineer`, `cloud design engineer`, the silicon
block), the product-side markers (`product design engineer`, `ux/ui/web design
engineer`, `design engineer, product`, `service/experience/sound/game design
engineer`), and then the draughting aliases themselves. The unqualified `design
engineer` closes the block and resolves to `engineering_design`: on this catalogue
that population is overwhelmingly mechanical.

Titles where a category alias appears but names no category at all — "Software Design
Engineer" is software engineering — carry the `categoryNone` sentinel as their table
entry, and `matchCategory` serves it as `""`. Deliberately NOT a pre-match mask like
`gradeBlindPhrases`: cutting the span exposes the aliases further down the table (which
are mostly the business categories, so "Software Design Engineer - Sales Tools" read as
`sales`) and is boundary-blind. Every exit translates the sentinel — `Parse`,
`Categories`, `CategoryAliases` — because the last two feed a CV profile and the
generated web contracts.

## What the ingest filter refuses

The catalogue is broader than IT. The ingest catalogue filter refuses a crawled posting
only through `ConfirmedNonTech`: its title names a role from the non-tech title list
(`nontech.go`) AND nothing places it — no technical evidence, and **no category of any
kind**. A resolved category, technical or not, is a deliberate placement: the posting is
kept, filed under that facet, and only `is_tech = false` follows. A non-technical title
the dictionary cannot FILE is refused, because a posting no facet reaches is one search
excludes anyway (`search.CategoryUnresolved`).

So growing the catalogue is a dictionary change, not a filter change: give the role a
category and the next crawl admits it. Every term in the non-tech list that names a job
outright is filed in `categoryTable` — in a block declared after every technical block,
so a title some alias above already resolves keeps exactly the category it had. As of
`social_services`, `science` and `manufacturing` every term in that list is filed, so the
refusal is now reached only by a title an earlier `categoryNone` pin claims. A new
non-tech term with no category to file it under is a category waiting to be added, not
an exception to carve out.

## Social services, science, manufacturing

Named after their SOC major groups — 21 (community and social service), 19 (life,
physical and social science), 51 (production) — all `vocab.NonTechCategories`, all
declared at the very END of `categoryTable`, after the media crafts, so they only reach a
title nothing else resolves. The one deliberate move is the production-floor spellings
(`production operator/associate/supervisor`, `machine operator`), re-filed from
`skilled_trades` in place; the heavy-equipment operator stays a trade. There is no bare
`scientist` and no bare `research associate`: "Research Scientist", "Applied Scientist"
and "Research Associate" (equity research) are software, ML or finance as often as
laboratory, so only the discipline-named and laboratory spellings are aliases. The
clinical laboratory (`medical lab technician`, `clinical laboratory scientist`,
`histotechnologist`, `cath lab`) is declared first in the science block and resolves
`healthcare`, as SOC 29-2010 files it.

The hazard this moves: an alias that was harmless while the filter refused every
non-technical title can now admit one into a TECHNICAL category. "Front End Cashier"
(the supermarket checkout lane) resolved to `frontend` all along and nobody saw it,
because the cashier was turned away. Before adding a broad technical alias, run it
against the titles the filter used to refuse — the ingest log's `rejected N/M postings
as non-technical, e.g. ...` lines carry two per board.

## Central and Eastern Europe

Polish, Czech, Slovak, Slovenian, Croatian, Serbian, Ukrainian and Bulgarian titles have
their own block at the end of `categoryTable` (freehire#3513), behind every other block, so
it only reaches a title nothing above resolves. Two rules shape it:

- **Surface forms, not stems.** The matcher has no morphology, so a role noun is listed in
  its masculine and feminine forms, and a desk in the CASE titles put it in — Polish names
  the desk in the genitive ("ds. sprzedaży", "Działu Logistyki"), Slovenian and Croatian in
  the locative ("v Marketingu", "u proizvodnji"). The nominative would match neither.
- **Role noun before desk.** The table resolves in declaration order, not title order, so
  the role nouns are declared first and the desk nouns last: "Magazynier - obsługa
  produkcji" is a warehouse worker.

Each alias is filed where its English twin is, and what English leaves unresolved on
purpose stays unresolved here too: the generic specialist/associate nouns, the bare
quality, compliance and tax desks, interns and student work, talent pools.

Two nouns are handled with care, because a bare one shadows every title that names its
craft in a word the table does not know (freehire#3518):

- **There is no bare engineer noun** (inženir, inżynier, inženjer, inženýr, inžinier,
  інженер), as there is none in English. Only the discipline-named spellings resolve —
  "procesni inženir" is the plant, "sistemski inženir" software, "prodajni inženir"
  solutions engineering, "radiološki inženir" healthcare.
- **The bare programmer nouns are declared LAST in the block.** They are software, the way
  "программист" is, but only once nothing else in the title has spoken: "CNC operater -
  programer" is the machine operator and "Tehnolog - programer" the technologist. The CNC
  programmer is filed on the floor (SOC 51-9162); the PLC/CAM/CAD/robot programmers are
  pinned to no category.

The Slovenian vocabulary inside the block was contributed in #3518 from 3,288 Slovenian
titles and curated: catch-all nouns and civil-service ranks were dropped, entries were
re-filed to their English twin, and a phrase that its own shorter aliases already resolve
was pruned.

## There is no management category

`management` was retired. It held every title whose only resolved word was "manager" —
about 6% of the open catalogue, a burger chain's Assistant Manager beside a Tax Manager
and a Case Manager — and told a jobseeker nothing about the work. Two axes replace it:

- **The craft**, in this table. A manager title resolves what it manages: Restaurant
  Manager `hospitality`, Store Manager `retail`, Plant Manager `manufacturing`, Tax and
  Relationship Manager `finance`, Construction/Site Manager `skilled_trades`, IT Manager
  `devops`, Engineering Manager `software_engineering` (the discipline-named "Mechanical/
  Project/Plant/... Engineering Manager" spellings are pinned to `industrial_engineering`
  ahead of it). Those entries sit in the managers block at the END of the table, so a
  title any earlier alias resolves keeps its category.
- **Whether it manages people**, in `internal/dict/roletype` (the `role_type` facet).
  General Manager, Assistant Manager, Store/Branch/District/Area/Plant/Shift Manager and
  the Russian начальник/заведующий/директор name no craft, so no category is true for
  them; roletype marks them `people_manager`, and search keeps a posting that facet
  reaches (`search.CategoryUnresolved`). The ingest refusal is vetoed the same way
  (`ConfirmedNonTech`), so the two gates agree.

There is no bare "manager"/"менеджер" alias. A manager title naming neither a craft nor
a people marker — Case Manager, Portfolio Manager, a bare "Senior Manager, X" — resolves
nothing and is excluded from search; measured on a 2% sample of the open catalogue at
retirement, that was about 2.9% of all postings (half of what `management` held). A
stored `category=management` in a saved search or link is answered as
`role_type=people_manager` by the search filter.

## Public safety

`public_safety` is SOC major group 33: police, sheriffs, corrections, firefighters,
security guards, lifeguards, crossing guards, loss prevention. Non-technical. It is never
information security, and three placements keep the two apart:

- The guard spellings ("Security Guard", "Security Officer") sit in the security block,
  behind the infosec officer spellings that contain them ("Information Security Officer",
  "Information Systems Security Officer", "IT/Cyber/Data Security Officer"), declared as
  whole phrases ending in "officer". The defence-industry "Program/Facility Security
  Officer" names neither craft and carries `categoryNone`.
- The emergency dispatchers and the firefighter sit before the logistics block (whose
  bare "dispatcher" would take the first) and the healthcare "paramedic" (which would
  take "Firefighter/Paramedic"). EMT and paramedic alone stay `healthcare`.
- Police, sheriffs, corrections and loss prevention sit at the END of the table: those
  words name the EMPLOYER as often as the job ("Food Service Worker - Sheriff's Office"
  is a cook). The ВОХР "стрелок" is last of all, behind a `categoryNone` pin for the
  military contract posting ("военнослужащий") that also names a rifleman.

The protective-services pins the HSE block used to carry as `categoryNone` ("Public/
Campus/School Safety Officer") now route here — an exclusion must route, not blank.

## `Categories` reads each span once

`Categories` returns every category the text mentions, so a pin that holds for `Parse`
used to leak through it ("Front End Cashier" came back `[retail, frontend]`). An alias
occurrence lying wholly inside a span an EARLIER alias already matched is now not
counted (`matchAllOrdered`). That is what lets "Security Officer" resolve the guard
category without tagging a CISO's CV with it: the whole phrase "information security
officer" claims the span first. A shorter alias that also occurs outside every claimed
span still counts.

## The one place category and `is_tech` disagree on purpose

IT support (`service desk`, `help desk`, `helpdesk`, `it support`, `it supporter`,
`desktop support`, `deskside support`, `end user support`, `technical support
analyst`) resolves the `support` category — a `vocab.NonTechCategories` member — and is *also* a
`techTitleTerms` entry, so `jobderive.deriveIsTech` reads it as `is_tech = true`.
That is deliberate, not drift: the category is right about the FUNCTION (reactive,
ticket-driven, alongside customer service) and wrong about the CRAFT, and only the title
list can say the second thing. `tech.go` carries the argument and the measurements
beside the terms; do not restate them here, or the two copies will drift.

**A term earns its place by naming the ESTATE, not the mood of the work.** That is why
bare `technical support` has no entry — live titles give AGV, automotive, controls,
logistics and instructional support under that phrase — while `desktop support` does.
If you are tempted to widen this family, sample the phrase against live titles first;
every term here was admitted that way, and the whole exception depends on none of them
being loose.

## Serving: dict-only

`jobview.FromRow` overwrites the nested `enrichment.seniority`/`enrichment.category` with the `jobs` column — the dictionary always wins, the LLM's value is never a fallback. They remain **nested under `enrichment`** so existing search facets, SPA, and generated contracts are unchanged.

## Convention

- Adding a value: add it to `vocab.SeniorityValues`/`vocab.CategoryValues` and the title-matching dictionary.
- Dictionary change needs `cmd/backfill-derive` + `cmd/reindex` to reach existing jobs (same caveat as geography/skills).

## Owed right now

Two changes landed on 2026-09-15 that only reach postings written after them, and neither
has had its backfill run. **Delete this section once it has.**

- **#2847** replaced the bare `security` category alias with qualified forms. Measured
  over the 150 commonest live titles carrying the word (10,276 open postings): 10,156 read
  as technical before, 5,917 after.
- **#2849** made an explicit non-tech TITLE outvote a technical category resolved from a
  substring (`jobderive.deriveIsTech`). Measured over the 3,000 commonest titles flagged
  technical (197,916 open postings): 22 titles and 2,067 postings move, every one a
  correction.

Together roughly **6,300 stored postings read `is_tech = true` against what the current
dictionaries say** — mall guards, nurses, HVAC project managers, occupational safety
inspectors.

**An open posting corrects itself on its next crawl, and only that.** `job.New` runs
`jobderive.Derive` on every ingest, and `is_tech` is part of `RefreshUnchangedJob`'s match
key, so a row whose derived value moved fails the cheap refresh and goes through
`UpsertJob`, which writes the new one. What the backfill is for is everything that will
NOT be crawled again: **closed postings above all** — they appear in no listing, so
nothing re-derives them — and any posting whose board or provider has stopped being
crawled. Until then those stay in technical search, and `cmd/search-ping` spends a little
of Google's 200-a-day allowance on the open ones it reaches first (measured 2026-09-15:
14 of 398 announcements, 3.5%).

Clearing it is `cmd/backfill-derive` (~15h; hold `BACKFILL_CONCURRENCY` at 2-3, it has
degraded prod at 6) followed by a full `make reindex` — `is_tech` is not part of
`content_hash`, so an incremental push alone never reaches these rows.
