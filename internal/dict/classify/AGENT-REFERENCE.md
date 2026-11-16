# internal/dict/classify — Seniority & Category Tagging

Deterministic seniority/category tagging from job title, feeding enrichment facets.

## Design

- Parses the **job title** at ingest into canonical `jobs.seniority`/`jobs.category` columns.
- Values from `vocab.SeniorityValues`/`vocab.CategoryValues` — EN+RU aliases, whole-word matched. Russian forms are full surface forms (not stems): the matcher requires word boundaries. **Never guesses**.
- Same alias→canonical dictionary design as `internal/dict/location` and `internal/dict/skilltag`.
- The main table resolves in declaration order, not title order. When it resolves nothing, the fallbacks run in this order (see `classify.go`): German, Dutch, Danish, Spanish, Portuguese, French, Polish, BCS, Slovenian. A `categoryNone` veto at any step ends the chain. Seniority reads `japaneseSeniorityTable`, `chineseSeniorityTable` and then `indianSeniorityTable` first, then the same chain. A fallback never changes a title an earlier table resolved.

## Grade-blind phrases

Some titles contain a grade word without stating a grade: "Member of Technical Staff", "Lead Generation", "Middle East", "Mid-training", "Assistant/Associate Vice President" (an AVP is below VP, not `c_level`). `gradeBlindPhrases` replaces each with a space **before** the seniority match. Ordering cannot fix this (`staff` outranks `senior`, so "Senior Member of Technical Staff" read `staff`); "Senior Staff Engineer" is still `staff`. The list holds only phrases that shadow a `seniorityTable` alias; the category match reads the untouched title.

## UK early-careers vocabulary

"Graduate" is `junior`; placement phrases ("Industrial Placement", "Year in Industry", "Summer Placement", "Vacation Scheme", ...) are `intern`. `graduate` is the LAST seniority entry (weakest evidence), so "Graduate Summer Internship" stays intern and any explicit grade outranks it. Bare "placement" (recruiters on prod) and bare "undergraduate" (faculty/admissions titles) are deliberately absent. Desks that merely contain the words ("Graduate Admissions", "Graduate Recruiting Manager", "Work Placement Officer") are cut before the match by `earlyCareersDeskPatterns` — a shape (the word beside the desk it names), not a title list.

## Japanese (and the CJK boundary)

Japanese writes no spaces (シニア+データサイエンティスト). `titleBoundary` (cjk.go) governs every title alias: a kana/kanji alias edge needs no boundary on that side, and a CJK neighbour counts as a boundary for a Latin edge ("社内ITサポート"). Latin-beside-Latin keeps the word boundary. Chinese shares the rule.

`japanese.go` has two slices around the core table: technical compounds LEAD it (Japanese titles carry Latin fragments the core table would claim); non-IT professions and broad business heads TRAIL it (a bilingual title is read by its English half first). Within each, profession > function word, and domain words (介護, ホテル) come last. A bare エンジニア/データ/開発/デザイナー is never an alias — classify_ja_test.go keeps the titles that prove why.

`chinese.go` (Simplified and Traditional listed separately) is LAST in `categoryTable`, so anything already resolved keeps its answer and Japanese is read as Japanese first. Every alias is a Han-bearing role compound — never a bare 工程師/人員/助理/開發/設計, never purely Latin (`TestChineseAliasesCarryHan`). Unresolved Japanese titles fall through to it, so a kanji form whose meaning differs between the languages (保全, 業務, 行政) is listed only in Chinese compounds or vetoed; classify_zh_test.go keeps the proving Japanese titles.

## The two design crafts

`design` is product/visual/experience design. Engineering draughting (mechanical, electrical, civil, architectural/BIM) is `engineering_design`, a `vocab.NonTechCategories` member. Silicon design is `hardware`.

Three groups sit **before** the bare `designer`/`design` entries, or `design` would claim every "… Design Engineer": titles naming another craft (`network/cloud design engineer`, the silicon block), product-side markers (`product/ux/ui/web design engineer`, `design engineer, product`, `service/experience/sound/game design engineer`), then the draughting aliases. The unqualified `design engineer` closes the block as `engineering_design` (overwhelmingly mechanical here).

A title where an alias appears but names no category ("Software Design Engineer") carries the `categoryNone` sentinel; `matchCategory` serves it as `""`. It is deliberately NOT a pre-match mask like `gradeBlindPhrases`: cutting the span exposes aliases further down (mostly business categories — "Software Design Engineer - Sales Tools" read `sales`) and is boundary-blind. Every exit translates the sentinel — `Parse`, `Categories`, `CategoryAliases` (the last two feed CV profiles and the generated web contracts).

## No ingest filter on title

Ingest no longer refuses a posting for its title (#4412; `ConfirmedNonTech` is gone). A title no category files is stored and indexed under `other` ("Unclassified"). Growing the facets is still a dictionary change: give the role a category and the next crawl files it.

**The hazard:** a broad technical alias can admit a non-tech title into a TECHNICAL category ("Front End Cashier" resolved to `frontend`). Before adding one, run it against the `other` bucket's titles.

## Social services, science, manufacturing

SOC major groups 21, 19 and 51, all `vocab.NonTechCategories`, declared at the very END of `categoryTable` after the media crafts. The production-floor spellings (`production operator/associate/supervisor`, `machine operator`) were re-filed from `skilled_trades` in place; the heavy-equipment operator stays a trade. No bare `scientist` or `research associate`: "Research/Applied Scientist" and "Research Associate" (equity research) are as often software, ML or finance, so only discipline-named and laboratory spellings are aliases. The clinical laboratory (`medical lab technician`, `clinical laboratory scientist`, `histotechnologist`, `cath lab`) is first in the science block and resolves `healthcare` (SOC 29-2010).

"Back end" (measured 2026-10-29 over the open `backend` population) is a SHIFT in US plants/hospitals/care homes ("Registered Nurse - Back End Days") and an AREA in can plants, body shops and stores ("Back End Operator", "Back-End Estimator", "Backend Retail Assistant"):

- The shift names no category, so `backEndShiftPatterns` CUTS it before the category walk — the one category-side mask, the opposite case from `categoryNone` (here exposing the aliases below is the point). It needs "back end" directly beside a day/night/shift/relief word, so "Backend Developer - Night Shift" keeps its reading.
- The area is PINNED to its English twin's category ahead of the bare alias (the "front end cashier" shape). Bare "back-end associate" is not pinned: "associate" names no category.

## Central and Eastern Europe

Polish, Czech, Slovak, Slovenian, Croatian, Serbian, Ukrainian and Bulgarian titles have a block at the end of `categoryTable` (freehire#3513), behind every other block.

- **Surface forms, not stems.** No morphology: role nouns in masculine and feminine forms, desks in the CASE titles use — Polish genitive ("ds. sprzedaży", "Działu Logistyki"), Slovenian/Croatian locative ("v Marketingu", "u proizvodnji").
- **Role noun before desk:** role nouns declared first, desk nouns last ("Magazynier - obsługa produkcji" is a warehouse worker).
- Each alias is filed with its English twin; what English leaves unresolved stays unresolved (generic specialist/associate nouns, bare quality/compliance/tax desks, interns and student work, talent pools).
- **No bare engineer noun** (inženir, inżynier, inženjer, inženýr, inžinier, інженер), as in English (freehire#3518) — only discipline-named spellings ("procesni" plant, "sistemski" software, "prodajni" solutions engineering, "radiološki" healthcare).
- **Bare programmer nouns are LAST in the block**: software only once nothing else spoke ("CNC operater - programer" is the machine operator, "Tehnolog - programer" the technologist). The CNC programmer is filed on the floor (SOC 51-9162); PLC/CAM/CAD/robot programmers are pinned to no category.

Slovenian vocabulary came from 3,288 titles (#3518), curated: catch-all nouns and civil-service ranks dropped, entries re-filed to the English twin, phrases already resolved by shorter aliases pruned. Croatian/Serbian came as 3,512 titles (#3778), ~1 in 5 of the unread ones mis-filed against the English twin; kept are the role nouns in both standards (ijekavian savjetnik, ekavian savetnik). "Predavač" is the Slovak shop assistant and the Croatian lecturer, so only "predavačica" is education. "Projektant" and "arhitekt" sit after the programmer nouns ("projektant programer" stays software). Measured 2026-10-28: category 66.8% → 68.8% over 12,945 hr/rs/ba/me postings; 69 of 44,130 regional controls changed, all hand-checked. A third pass (#4412) filed the six regional sources' unresolved heads (teaching and care aides, allied health degrees, farm/forest/sawmill/construction trades, shop counters, real estate, civil protection) plus a few English twins after the bare "support"; the farm trades follow "kmetijski delavec" to `skilled_trades`; boiler operator, waste work, bare administrator stay unresolved. Branch, store, department, shift and group heads and the director name no craft: `roletype` marks them `people_manager` ("voditelj/šef/vodja" only before a workplace noun, the director in the nominative or after a deputy), which keeps 199 more of the 2,847 still unresolved (7.0%; optius 16.4%, mojedelo 14.7%, mojposao 10.4%, ess 9.5%, infostud 3.9%, hzz 3.0%); 3 of 80,048 controls gained the mark, none lost or changed one. Measured 2026-11-10: 700 of 3,547 unresolved open postings now resolve (hzz 33.5%, infostud 13.4%, others 4–8%); 50 of 80,048 controls changed, none that resolved before, all hand-checked.

## There is no management category

`management` was retired (~6% of the catalogue, a burger chain's Assistant Manager beside a Tax Manager — said nothing about the work); do not reintroduce it. Two axes replace it:

- **The craft**, here: a manager title resolves what it manages (Restaurant `hospitality`, Tax `finance`, IT `devops`, Engineering Manager `software_engineering`; discipline-named "Mechanical/Project/Plant/... Engineering Manager" pinned to `industrial_engineering` ahead of it). These sit in the managers block at the END of the table.
- **Whether it manages people**, in `internal/dict/roletype` (`role_type` facet). General/Assistant/Store/Branch/District/Area/Plant/Shift Manager and начальник/заведующий/директор name no craft; roletype marks them `people_manager`.

There is no bare "manager"/"менеджер" alias. A manager title naming neither craft nor people marker (Case Manager, Portfolio Manager, "Senior Manager, X") resolves no category and lands in `other` (findable by keyword) — ~2.9% of postings on a 2% sample at retirement. A `category=management` filter value (saved searches, old links) is answered as `role_type=people_manager` by the search filter.

## Public safety

`public_safety` is SOC 33 (police, sheriffs, corrections, firefighters, guards, lifeguards, crossing guards, loss prevention), non-technical, never infosec:

- "Security Guard"/"Security Officer" sit in the security block behind the infosec officer phrases that contain them ("Information Security Officer", "Information Systems Security Officer", "IT/Cyber/Data Security Officer"). "Program/Facility Security Officer" (defence industry) carries `categoryNone`.
- Emergency dispatchers and firefighter sit before the logistics block (bare "dispatcher") and healthcare "paramedic" ("Firefighter/Paramedic"). EMT and paramedic alone stay `healthcare`.
- Police, sheriffs, corrections and loss prevention sit at the END: they name the EMPLOYER as often as the job ("Food Service Worker - Sheriff's Office" is a cook). ВОХР "стрелок" is last, behind a `categoryNone` pin for "военнослужащий".

"Public/Campus/School Safety Officer" route here, not to an HSE `categoryNone` pin — an exclusion must route, not blank.

## `Categories` reads each span once

`Categories` returns every category mentioned, but an alias occurrence wholly inside a span an EARLIER alias matched is not counted (`matchAllOrdered`), so pins hold there as in `Parse` ("Front End Cashier" is not `[retail, frontend]`; "information security officer" claims its span before the guard alias). A shorter alias that also occurs outside every claimed span still counts.

## IT support is its own category

Help/service desk, desktop/end-user support, tiered desk (1st/2nd line, L1-L3), technical support engineer/analyst/specialist, application support and NOC resolve `it_support` (a `vocab.TechCategories` member, so `is_tech = true`). `support` keeps customer service, call/contact centre, customer care, non-technical. `techTitleTerms` still lists the IT terms because a title term wins outright where a category yields to a non-tech word (`tech.go` explains).

**A phrase must name the ESTATE, not the mood of the work.** Bare `technical support` and `support engineer` stay `support` (AGV, field-equipment, maintenance support); "Tier 1" is absent (heads customer-service titles as often). Sample a phrase against live titles with `TestCategoryCorpusProbe` before widening.

**Bare `support` is the LAST entry of `coreCategoryTable`**: it names nothing as often as a desk ("Associate Dentist – Full Support", "Autism Support Specialist (RBT)"), and beside "customer service" it outranked every later profession. Customer-side phrases ("customer service/support", "client support", "call center") stay in the support block ahead of the professions. Measured 2026-10-26 on a 10% sample (22,638 postings): 3,057 → `it_support`, ~1,500 → their profession, ~18,100 stay.

## German compound heads

German names the role in the LAST morpheme (Kranken\|pfleger, Stapler\|fahrer). `german.go` holds `germanCategoryTable` and `germanSeniorityTable`: an alias `-pfleger` matches any word ending in it, one without the dash is a whole-word phrase.

Heads resolve per WORD: a word takes the first head it ends in, so a longer veto (`-begleiter` → `categoryNone`) claims "Alltagsbegleiter" before `-leiter` reads a lead, without blanking the rest of the title. Feminine endings (`-in`, `-innen`) are folded once per word; `(m/w/d)` notices are cut first. Trainee words outrank `-leiter`/`-leitung` ("Praktikant Bauleitung" is intern). Short or ambiguous words are absent: `maler` is whole-word only ("normaler"); `MFA`/`OTA`/`PTA` mean other things in English.

Measured 2026-10-25 over 123,605 postings (coveto, rexx, xing, dvinci, concludis, onlyfy, kenjo): category 15.4% → 75.3%, seniority 2.0% → 24.6%; a 100-title hand-checked hold-out found one debatable category and one debatable grade.

Invisible format characters (U+00AD soft hyphens above all) are cut by `normalize.CleanTitle` inside `jobderive.Derive`, for the dictionaries only — the stored title and public slug keep the source text.

## Dutch compound heads

`dutch.go` (`dutchCategoryTable`, `dutchSeniorityTable`) uses the German syntax and matcher (`matchHeads`). Trade-less heads (`medewerker`, `adviseur`, `coördinator`, `specialist`) are absent; only the trade-naming compound resolves ("magazijnmedewerker", "HR adviseur"). `medior` is `middle`; plain `directeur` states no grade; `algemeen directeur`, `directeur-bestuurder`, `financieel/technisch directeur` are `c_level`. The fallback also sees English, French and German, so `stage` is whole-word only ("Urlaubstage", "Justage") with its English uses vetoed, and `-kok` vetoes "Bangkok".

Measured 2026-10-25 over 42,176 Dutch postings (eures, recruitee, whatjobs-nl): category 54.9% → 72.8%, seniority 22.0% → 27.4%; 542k control titles hand-checked: the new readings are almost all Dutch/Flemish/French and correct.

## Indian grade forms

`india.go`'s `indianSeniorityTable` is read after the main table, so a stated grade wins ("Senior Software Engineer II" is senior). The SDE ladder (and SDET, "Software Development Engineer") reads SDE-1 junior, SDE-2 middle, SDE-3 senior; other ladders read only Roman I and II ("Software Engineer III" varies by employer; "Java Developer 2-5 Years" is a range). "Associate" grades only the role it precedes ("Associate Software Engineer"). SMTS/LMTS/PMTS are graded, bare MTS is not. Fresher junior, Apprentice intern, Team Leader lead.

In the MAIN table: trainee HIRES ("Graduate Engineer Trainee", "Management Trainee", "Graduate Trainee", "Trainee Engineer") are junior, ahead of bare "trainee" (intern). No grade is ever read from a years figure. Measured 2026-10-25 over 63,305 Indian postings: seniority 31.6% → 34.1%; no wrong reading in the top 400 newly resolved.

## Spanish heads and fields

Moved to [AGENT-REFERENCE-LANGUAGES.md](AGENT-REFERENCE-LANGUAGES.md#spanish-heads-and-fields).

## Portuguese heads and fields

Moved to [AGENT-REFERENCE-LANGUAGES.md](AGENT-REFERENCE-LANGUAGES.md#portuguese-heads-and-fields).

## French heads and fields

Moved to [AGENT-REFERENCE-LANGUAGES.md](AGENT-REFERENCE-LANGUAGES.md#french-heads-and-fields).

## Polish stems

Moved to [AGENT-REFERENCE-LANGUAGES.md](AGENT-REFERENCE-LANGUAGES.md#polish-stems).

## Croatian and Serbian heads and fields

Moved to [AGENT-REFERENCE-LANGUAGES.md](AGENT-REFERENCE-LANGUAGES.md#croatian-and-serbian-heads-and-fields).

## Slovenian grades and fields

Moved to [AGENT-REFERENCE-LANGUAGES.md](AGENT-REFERENCE-LANGUAGES.md#slovenian-grades-and-fields).

## Serving: dict-only

`jobview.FromRow` (through `FromDomain`) overwrites nested `enrichment.seniority`/`enrichment.category` with the `jobs` column — the column always wins, the LLM value is never a fallback. That column is `jobderive.Derive`'s answer: a source's structured value first, then the title dictionary, then the description. They stay **nested under `enrichment`** so search facets, SPA and generated contracts are unchanged.

## Convention

- Adding a value: add it to `vocab.SeniorityValues`/`vocab.CategoryValues` and the title-matching dictionary.
- Dictionary changes need `cmd/backfill-derive` + `cmd/reindex` to reach existing jobs (same as geography/skills).

## Owed right now

Two 2026-09-15 changes reach only postings written after them; neither backfill has run. **Delete this section once it has.**

- **#2847** replaced the bare `security` alias with qualified forms: of the 150 commonest titles with the word (10,276 open postings), 10,156 read technical before, 5,917 after.
- **#2849** made an explicit non-tech TITLE outvote a technical category resolved from a substring (`jobderive.deriveIsTech`): over the 3,000 commonest technical titles (197,916 postings), 22 titles / 2,067 postings move, all corrections.

So ~**6,300 stored postings wrongly read `is_tech = true`**.

An open posting re-read in full corrects itself on its next crawl: `job.New` runs `jobderive.Derive`, and `is_tech` is in `RefreshUnchangedJob`'s match key, so a moved row goes through `UpsertJob`. Not on a hydrating source (`FetchNew`: `workday`, `smartrecruiters`, ...): it re-lists a seen posting as `SeenRefresh`, which the pipeline only `touch`es. The backfill is for those rows and for rows never crawled again — **closed postings above all**, and boards/providers no longer crawled. Until then those stay in technical search, and `cmd/search-ping` spends some of Google's 200/day allowance on the open ones it reaches first (2026-09-15: 14 of 398 announcements, 3.5%).

Fix: `cmd/backfill-derive` (~15h; `BACKFILL_CONCURRENCY` 2-3, 6 degraded prod) then a full `make reindex` — `is_tech` is not in `content_hash`, so an incremental push never reaches these rows.
