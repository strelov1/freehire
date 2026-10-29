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

## Japanese (and the CJK boundary)

Japanese writes no spaces, so a role compound arrives glued to its qualifier
(シニア+データサイエンティスト). `titleBoundary` (cjk.go) is the rule every title alias is matched
under: an alias edge that is kana or kanji needs no boundary on that side, and a CJK
neighbour counts as a boundary for a Latin edge ("社内ITサポート"). Latin-beside-Latin keeps the
ordinary word boundary. Chinese shares the rule.

`japanese.go` holds the aliases in two slices around the core table: technical compounds
LEAD it (a Japanese title carries Latin fragments the core table would claim), the
non-IT professions and broad business heads TRAIL it (a bilingual title is read by its
English half first). Inside each, a profession outranks a function word and a domain
word (介護, ホテル) comes last. A bare エンジニア/データ/開発/デザイナー is never an alias —
classify_ja_test.go keeps the real titles that prove why.

`chinese.go` holds the Chinese aliases (Simplified and Traditional, listed separately),
LAST in `categoryTable`, so a title anything else resolves keeps its answer and a Japanese
title is read as Japanese first. Every alias is a role compound carrying Han — a bare
工程師/人員/助理/開發/設計 is never one, and a purely Latin alias would reach Latin titles
(`TestChineseAliasesCarryHan`). Japanese titles the Japanese blocks leave unresolved still
fall through to it, so a kanji form whose meaning differs between the languages (保全, 業務,
行政) is listed only in its Chinese compounds or vetoed; classify_zh_test.go keeps the
Japanese titles that prove each one.

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

The Croatian and Serbian vocabulary was contributed in #3778 as 3,512 whole titles; about
one in five of those the table did not already read was filed away from its English
twin. What was kept is the role noun or phrase inside them, in both standards (ijekavian
savjetnik, ekavian savetnik), filed where its English twin is; the catch-all nouns were
dropped as in #3518. Two nouns collide across
languages: "predavač" is the Slovak shop assistant and the Croatian lecturer, so only the
Croatian feminine "predavačica" is read as education. "Projektant" and "arhitekt" sit
after the programmer nouns, so "projektant programer" stays software. Measured 2026-10-28:
category 66.8% → 68.8% over 12,945 open hr/rs/ba/me postings; over 44,130 control postings
from the rest of the region, 69 changed and all were hand-checked. Most were Polish,
Czech and Slovak "projektant" titles (road, bridge and building-services design), now
read as engineering_design.

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

## IT support is its own category

The help desk, the service desk, desktop/deskside/end-user support, the tiered desk
(1st/2nd line, L1-L3), the technical support engineer/analyst/specialist, application
support and the NOC resolve `it_support` — a `vocab.TechCategories` member, so the
category alone derives `is_tech = true`. `support` keeps customer service, the call and
contact centre and customer care, and stays non-technical. Until 2026-10 both sat in
`support`, and the IT half reached `is_tech = true` only through `techTitleTerms`; those
terms stay, because a title term wins outright where a category yields to a non-tech
word elsewhere in the title (`tech.go` carries the argument beside them).

**A phrase earns its place by naming the ESTATE, not the mood of the work.** Bare
`technical support` and bare `support engineer` stay `support`: live titles give AGV,
automotive, field-equipment, robotics and maintenance support under them. "Tier 1" is
absent too — it heads customer-service titles as often. Widen the family only after
sampling the phrase against live titles; `TestCategoryCorpusProbe` is the tool.

**The bare `support` is the LAST entry of `coreCategoryTable`.** It names a desk as often
as nothing at all ("Associate Dentist – Full Support", "Mortgage Loan Officer –
Purchase-Focused, Full Support", "Autism Support Specialist (RBT)"), and while it sat
beside "customer service" it outranked every profession declared after it. The phrases
that name the customer side outright ("customer service", "customer support", "client
support", "call center") stay in the support block, ahead of the professions. Measured
2026-10-26 over a 10% sample of the open `support` population (22,638 postings): 3,057
move to `it_support`, ~1,500 to the profession their title names, ~18,100 stay.

## German compound heads

German names a role in a compound whose LAST morpheme is the role — Kranken|pfleger,
Stapler|fahrer — so whole-word aliases cannot keep up with the long tail. `german.go`
holds two tables, `germanCategoryTable` and `germanSeniorityTable`, where an alias
written `-pfleger` matches any word ending in it and an alias without the dash is a
whole-word phrase. They run only when the main table resolved NOTHING, so they never
change a title that already resolves.

A head resolves per WORD: a word takes the first head it ends in, so a longer veto
(`-begleiter` → `categoryNone`) claims "Alltagsbegleiter" before `-leiter` can read it
as a lead, without blanking the rest of the title. The fused feminine endings
(`-in`, `-innen`) are folded once per word; `(m/w/d)`-style notices are cut first so
they cannot split a phrase. Trainee words outrank the `-leiter`/`-leitung` heads: a
"Praktikant Bauleitung" is an intern.

Measured 2026-10-25 over the 123,605 open postings of coveto, rexx, xing, dvinci,
concludis, onlyfy and kenjo: category 15.4% → 75.3%, seniority 2.0% → 24.6% (most
German titles state no grade). A hand-checked hold-out of 100 newly resolved titles
found one debatable category and one debatable grade. Short or ambiguous words are
deliberately absent — `maler` is whole-word only because "normaler" ends in it, and
the acronyms `MFA`/`OTA`/`PTA` are out because English titles use them for other things.

The title's invisible format characters (U+00AD soft hyphens above all) are cut by
`normalize.CleanTitle` inside `jobderive.Derive`, for the dictionaries only — the
stored title and the public slug keep what the source sent.

## Dutch compound heads

Dutch builds role names the same way (servicedesk|medewerker, wijk|verpleegkundige),
so `dutch.go` holds `dutchCategoryTable` and `dutchSeniorityTable` with the same alias
syntax and the same matcher (`matchHeads`). They run after the main AND the German
tables resolved nothing. Generic heads that name no trade — `medewerker`, `adviseur`,
`coördinator`, `specialist` — are absent; only the compound or phrase that names the
trade resolves ("magazijnmedewerker", "HR adviseur"). `medior` is `middle`. A plain
`directeur` states no grade, as the main table's "director" does; `algemeen directeur`,
`directeur-bestuurder` and the CFO/CTO-style `financieel/technisch directeur` are
`c_level`.

Because the fallback also sees English, French and German titles, `stage` is a whole
word only — as a tail it read German "Urlaubstage" and "Justage" — and its English uses
(Stage Manager, Late Stage, Stage II, DataStage) are vetoed; `-kok` vetoes "Bangkok".

Measured 2026-10-25 over 42,176 open Dutch postings (eures, recruitee, whatjobs-nl;
`jobderive.Derive` on title, location and description only): category 54.9% → 72.8%,
seniority 22.0% → 27.4%. Over 542k control titles (greenhouse, lever, smartrecruiters,
workday, the German crawl) the Dutch tables resolved 2,814 categories and 2,149 grades,
hand-checked: almost all are Dutch, Flemish or French postings read correctly.

## Indian grade forms

`india.go` holds `indianSeniorityTable`, a FALLBACK read after the main table and before
the German and Dutch ones, so a stated grade always wins ("Senior Software Engineer II" is senior).
The SDE ladder reads three rungs (SDE-1 junior, SDE-2 middle, SDE-3 senior, and the SDET
and "Software Development Engineer" spellings); every other ladder reads only I and II,
in Roman numerals: "Software Engineer III" is senior at some employers and mid at others,
and "Java Developer 2-5 Years" is a range, not a rung. "Associate" grades only the role
it precedes ("Associate Software Engineer", "Associate Consultant"); alone, before a rank
or after a shop role it states nothing. SMTS/LMTS/PMTS are graded, bare MTS is not (it is
the grade-blind "Member of Technical Staff"). Fresher is junior, Apprentice intern, Team
Leader lead.

Two changes sit in the MAIN table, because the main table answered them wrongly: the
trainee HIRES ("Graduate Engineer Trainee", "Management Trainee", "Graduate Trainee",
"Trainee Engineer") are junior, ahead of the bare "trainee" that stays intern; and
"Assistant/Associate Vice President" are grade-blind phrases — an AVP is a corporate
title below VP, not an executive, and used to read as `c_level` through the "vice
president" inside it. No grade is ever read from a years figure in a title.

Measured 2026-10-25 over 63,305 Indian postings: title seniority 31.6% → 34.1%; a
hand-check of every newly resolved title in the top 400 found none wrong.

## Spanish heads and fields

Spanish titles are not compounds: they name a generic head and the field it works in
("Técnico de mantenimiento", "Auxiliar de cocina", "Ejecutivo de ventas"). `spanish.go`
holds `spanishCategoryTable`, whole-word phrases matched on a FOLDED title (accents
dropped, "/a", "-a", "(a)" and "(h/m)" cut), ordered in bands: a head and field whose
answer differs from the field's default ("Ingeniero de ventas" is pre-sales), then the
role nouns that name a trade alone (camarero, electricista, conductor), then the fields
(ventas, mantenimiento, cocina, almacén, limpieza), then the heads that name a desk alone
(administrativo, comercial). A generic head with no field — técnico, auxiliar, operario,
asesor, gerente, analista — resolves nothing. Before the Spanish table, the main table is
asked again of the gender-folded title, because "Operario/a de producción" hides an alias
it already carries. It runs after the German, Dutch and Danish fallbacks.

The bare "desarrollador"/"programador" resolve here, after vetoes the main table cannot
afford: the business developer is sales, the property and energy project developer
nothing, the CNC and production programmer manufacturing, the PLC, robot, maintenance
and service programmer nothing. "Médico" sits below the sales field (it is also the
adjective of the equipment a rep sells), "maestro" is absent (a foreman, a kitchen, a
restaurant chain), and a bare "bancario" is absent (a Portuguese banking lawyer).

Grades (`spanishSeniorityTable`): becario/beca/prácticas/practicante/pasante/aprendiz are
intern and outrank `jefe`, which is lead as the German `-leiter` is; responsable, gerente,
supervisor and coordinador state no grade, as manager and supervisor do in the main
table. The assistant OF a jefe is cut first. Only director general/ejecutivo/financiero
and consejero delegado are c_level. Latin America's "semi senior" is folded to `ssr` in
`matchSeniority` and is `middle` in the MAIN table (it contains "senior", so the fallback
could never reach it); listed after `sr`, the range "Ssr/Sr" keeps its upper bound.
Portuguese reaches the same fallback, so `estagio`/`estagiario` (intern) sit here too;
`pleno` moved to the Portuguese table below, after `sênior`.

Measured 2026-10-27 over 25,017 open Spanish-language postings (infojobs, occ,
pandapelatam, teamtailor, factorial, getonbrd, torre, hiringroom, personio, eures,
getmanfred; whatjobs excluded, it is already categorised): category 17.9% → 74.3%,
seniority 6.9% → 11.0%, and 74 semi-senior titles moved from senior to middle. Over
4,004 Portuguese postings (pandape, empregos): category 25.3% → 39.5%, seniority 1.8% →
17.4%. Over 81k control titles (whatjobs-uk/it/fr/pt, greenhouse, workday, coveto,
recruitee, jobindex, gupy, smartrecruiters) category 68.0% → 71.2%; every newly resolved
title was hand-checked and the misreadings found became vetoes.

**Argentina** (2026-10-27, 5,225 Spanish-language postings placed there; category 84.2% →
84.9%, seniority 23.1% → 23.6% — the Spanish table already covered most of it). Added the
rioplatense trades (`preventista`, `promovendedor`, `clarkista`, `cadete`,
`mucama`, `plegador`, `despachante de aduana`), "Gerente General" as c_level, and two
position rules: a title that OPENS with "Líder" is the lead (elsewhere the word praises the
employer, "Empresa Líder"), and an opening "Ssr" is the grade even beside Next.js (the
rendering guard used to cut it). Bare "Mozo/a" stays logistics: in Spain it is the
warehouse hand, in Argentina the waiter, and the title alone cannot tell them apart.

## Portuguese heads and fields

Portuguese titles have the Spanish shape — "Auxiliar de Serviços Gerais", "Analista de
Departamento Pessoal", "Operador/a de Armazém" — and words both languages spell alike
("vendedor", "comercial", "administrativo", "logística") are already read by the Spanish
table. `portuguese.go` holds `portugueseCategoryTable` for the rest, on the same folded
title and in the same bands, and it runs AFTER the Spanish table: no Spanish answer
changes, and a Spanish veto ("Programador de Manutenção") still blinds the title. It
lists both standards where they differ (equipe/equipa, recepcionista/rececionista,
empilhadeira/empilhador, European "restauração" for food service). A generic head with
no field — analista, auxiliar, assistente, técnico, operador, consultor, especialista,
engenheiro — resolves nothing; "analista" heads one Brazilian title in twelve.

Placement decisions with a reason: the lawyer outranks the tax field ("Advogado
Tributário" is legal); "fiscal" is the tax desk only after a desk head ("Fiscal de Loja"
guards a shop, "Fiscal de Obras" inspects a site); the sales field outranks the shop and
the service desk ("Consultor de Vendas - Loja", "Atendimento e Vendas" sell); "DP" (the
payroll desk) counts only as "de DP" (a bare DP is DP World or dynamic positioning);
"auxiliar de sala" is absent (Spanish waits tables in it); "desenvolvedor" alone is
software, after the business, product and property developer.

Grades (`portugueseSeniorityTable`, after the Spanish table): the accented
"Sênior"/"Júnior" the main table cannot see (it also reads the French "Sénior"),
`pleno` below `sênior` so "Pleno/Sênior" keeps its upper bound, `coordenador` and
`chefe` lead, the general/executive/president/financial directors c_level. `supervisor`
is left alone — spelled the same in Spanish and English, whose tables state no grade.
The Brazilian ladder (`brazilianGradeLadder`) reads "PL" and the Roman rungs (I junior,
II middle, III senior) only after a Portuguese desk head in the same clause, so
"PL/SQL", "Warsaw, PL" and a product's "II" state nothing.

Measured 2026-10-27 over 30,677 Brazilian postings (gupy, solides, quickin, catho,
empregos, vagas, inhire, recrutei, pandape, smartrecruiters, workday, whatjobs-br) and
6,931 Portuguese ones (smartrecruiters, teamtailor, workday, whatjobs-pt, recruitee,
personio, bamboohr, landingjobs): title category 43.1% → 77.9% (BR) and 64.0% → 75.7%
(PT); seniority 18.9% → 30.6% (BR) and 20.4% → 22.0% (PT). Over 17,500 control titles
(pandapelatam, infojobs, eures, personio, greenhouse, whatjobs-uk/it/fr) 77 changed,
all hand-checked (Spanish "logístico", French "Sénior", "RH").

## French heads and fields

French titles have the Spanish shape — "Technicien de maintenance", "Conseiller de vente",
"Chargé de recrutement", "Agent de quai" — with almost none of its words. `french.go`
holds `frenchCategoryTable` in the Spanish bands (head and field exceptions, role nouns,
fields, desk heads) and runs LAST, after the Portuguese table. Its aliases live in the
space `foldFrench` maps a title into: accents dropped, every gender notice cut ("(H/F)",
"F/H", "(e)", "·ne", ".e", "/euse") and each feminine word folded onto its masculine
("développeuse", "conseillère", "chargée", "commerciale"); `TestFrenchAliasesAreFolded`
holds every alias to that space. Before it the main table is asked again of the
gender-cut title ("Ingénieur·e DevOps").

It reads only a title that looks French (`looksFrench`: a gender notice, a French
accent, an elided article, a French function word, or a French-only head noun), because
many of its words are English too — commercial, assistant, agent, boutique, maintenance.
"Stage" is no such cue (English "Early Stage", Dutch "stage"). Workplace words (bank,
insurer, hotel, shop, haulier, laboratory) sit in `frenchWorkplaceTable`, consulted only
when the title names no technical work: on "Tech Lead Java - Transport" the transport is
the client's sector.

Two fixes reach the MAIN table's reading of French titles. "Chef de/d'/des X" is any
lead in French ("Chef de projet", "Chef d'équipe", "Chef de rayon"), but the main table's
`chef` is a cook: `unchefFrench` rewrites every non-kitchen one (partie, cuisine, rang,
salle, bar… keep it) to "cheffe" before the main walk — it was filing ~900 French titles
under hospitality. And the electrician's "CFO" (courants forts, beside CFA) is rewritten
on a French title that names electrical or building work, so "Technicien de maintenance
CFO/CFA" is no longer finance. The German `-monteur` gives way to the French video editor.

The bare "développeur" resolves here, after the business, property, territorial and
network developers are vetoed. A generic head with no field — technicien, assistant,
agent, chargé, responsable, gestionnaire, conseiller, consultant, ingénieur, directeur —
resolves nothing. Switzerland's "employé de commerce" is the office apprenticeship
(administration); Belgium's "technicien de surface" cleans; Québec's grocery clerk is
headed by the department ("Épicerie Commis").

Grades (`frenchSeniorityTable`): stage/stagiaire, alternance/alternant, apprenti and the
professionalisation contract are intern and outrank the lead; `confirmé` is middle;
`débutant` and `jeune diplômé` junior; `chef d'équipe`, `chef de groupe` and `responsable
d'équipe` lead. "Chef de projet", responsable, manager, directeur and expert state no
grade; only the general, executive and financial directors (and the PDG) are c_level.
The assistant of an executive and the desk that runs the trainee programmes
("Responsable alternance") are cut first.

Measured 2026-10-28 over 24,609 open French-language postings (wttj, flatchr, werecruit,
smartrecruiters, teamtailor, workday, eures, recruitee, lever, join, workable,
greenhouse, personio; whatjobs excluded, it is already categorised): category 20.5% →
72.8%, seniority 11.9% → 27.0%. Over 16,000 control titles (coveto, infojobs, gupy,
greenhouse, workday, paylocity, join, pandapelatam) 62 changed, all French, Spanish or
Portuguese titles read correctly. Still unresolved: a franchise's "Entrepreneur / City
manager immobilier" postings, "Boulanger" (English "Baker" resolves nothing either), and
generic heads with no field.

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
