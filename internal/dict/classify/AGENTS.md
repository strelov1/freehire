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

## What the ingest filter refuses

The ingest filter refuses a posting only through `ConfirmedNonTech`: the title names a role from the non-tech list (`nontech.go`) AND nothing places it — no technical evidence and **no category of any kind**. A resolved category, technical or not, keeps the posting under that facet with `is_tech = false`. An unfileable non-tech title is refused because search excludes it anyway (`search.CategoryUnresolved`).

So growing the catalogue is a dictionary change: give the role a category and the next crawl admits it. Every non-tech term that names a job is filed in `categoryTable`, in blocks declared after every technical block, so earlier answers do not move. The refusal is now reached only by titles an earlier `categoryNone` pin claims. A new non-tech term with no category is a category to add, not an exception to carve out.

**The hazard:** an alias harmless while the filter refused non-tech titles can now admit one into a TECHNICAL category ("Front End Cashier" resolved to `frontend` unseen, because cashiers were refused). Before adding a broad technical alias, run it against formerly refused titles — the ingest log's `rejected N/M postings as non-technical, e.g. ...` lines carry two per board.

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
- **Whether it manages people**, in `internal/dict/roletype` (`role_type` facet). General/Assistant/Store/Branch/District/Area/Plant/Shift Manager and начальник/заведующий/директор name no craft; roletype marks them `people_manager`, search keeps postings that facet reaches (`search.CategoryUnresolved`), and `ConfirmedNonTech` vetoes refusal the same way.

There is no bare "manager"/"менеджер" alias. A manager title naming neither craft nor people marker (Case Manager, Portfolio Manager, "Senior Manager, X") resolves no category, but `NamesManager` keeps it past both catalogue gates (`search.CategoryUnresolved`, `ConfirmedNonTech`): findable by keyword, reached by no facet — ~2.9% of postings on a 2% sample at retirement. A `category=management` filter value (saved searches, old links) is answered as `role_type=people_manager` by the search filter.

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

Spanish names a generic head plus field ("Técnico de mantenimiento", "Auxiliar de cocina"). `spanishCategoryTable` holds whole-word phrases on a FOLDED title (accents dropped; "/a", "-a", "(a)", "(h/m)" cut), in bands: head+field pairs whose answer differs from the field's ("Ingeniero de ventas" is pre-sales), then role nouns alone (camarero, electricista, conductor), then fields (ventas, mantenimiento, cocina, almacén, limpieza), then desk heads (administrativo, comercial). A field-less generic head (técnico, auxiliar, operario, asesor, gerente, analista) resolves nothing. The main table is first asked again of the gender-folded title ("Operario/a de producción").

Bare "desarrollador"/"programador" resolve here after vetoes: business developer sales; property/energy project developer nothing; CNC/production programmer manufacturing; PLC/robot/maintenance/service programmer nothing. "Médico" sits below the sales field (equipment reps); "maestro" (foreman, kitchen, chain) and bare "bancario" (Portuguese banking lawyer) are absent.

Grades (`spanishSeniorityTable`): becario/beca/prácticas/practicante/pasante/aprendiz intern, outranking `jefe` (lead); responsable, gerente, supervisor, coordinador state no grade; the assistant OF a jefe is cut first; only director general/ejecutivo/financiero and consejero delegado are c_level. "Semi senior" is folded to `ssr` in `matchSeniority` and is `middle` in the MAIN table (it contains "senior"); listed after `sr`, so "Ssr/Sr" keeps its upper bound. `estagio`/`estagiario` (intern) sit here too.

Measured 2026-10-27 over 25,017 Spanish postings (whatjobs excluded): category 17.9% → 74.3%, seniority 6.9% → 11.0%, 74 semi-senior titles senior → middle; over 4,004 Portuguese: category 25.3% → 39.5%, seniority 1.8% → 17.4%; over 81k control titles category 68.0% → 71.2%, every new title hand-checked, misreadings became vetoes.

**Argentina** (2026-10-27, 5,225 postings; category 84.2% → 84.9%, seniority 23.1% → 23.6%): rioplatense trades (`preventista`, `promovendedor`, `clarkista`, `cadete`, `mucama`, `plegador`, `despachante de aduana`), "Gerente General" c_level, a title that OPENS with "Líder" is lead (elsewhere "Empresa Líder" praises the employer), and an opening "Ssr" is the grade even beside Next.js. Bare "Mozo/a" stays logistics (Spain's warehouse hand vs Argentina's waiter).

## Portuguese heads and fields

`portuguese.go`'s `portugueseCategoryTable` uses the Spanish shape, folding and bands, for words Spanish does not already read ("vendedor", "comercial", "logística" are Spanish-table hits). It runs after Spanish, so no Spanish answer changes and a Spanish veto ("Programador de Manutenção") still blinds the title. Both standards are listed where they differ (equipe/equipa, empilhadeira/empilhador). Field-less heads (analista, auxiliar, técnico, operador, engenheiro, ...) resolve nothing — "analista" heads 1 in 12 Brazilian titles.

Placements: the lawyer outranks the tax field ("Advogado Tributário" is legal); "fiscal" is the tax desk only after a desk head ("Fiscal de Loja/Obras" guard/inspect); the sales field outranks shop and service desk ("Consultor de Vendas - Loja"); "DP" counts only as "de DP" (DP World, dynamic positioning); "auxiliar de sala" absent (Spanish waiting); "desenvolvedor" alone is software, after business/product/property developer.

Grades (`portugueseSeniorityTable`): accented "Sênior"/"Júnior", `pleno` below `sênior` ("Pleno/Sênior" keeps its upper bound), `coordenador`/`chefe` lead, general/executive/president/financial directors c_level; `supervisor` states no grade. `brazilianGradeLadder` reads "PL" and Roman rungs (I junior, II middle, III senior) only after a Portuguese desk head in the same clause, so "PL/SQL", "Warsaw, PL" and a product's "II" state nothing.

Measured 2026-10-27 over 30,677 BR / 6,931 PT postings: category 43.1% → 77.9% / 64.0% → 75.7%; seniority 18.9% → 30.6% / 20.4% → 22.0%; 77 of 17,500 control titles changed, all hand-checked.

## French heads and fields

`french.go`'s `frenchCategoryTable` uses the Spanish bands. Its aliases live in the space `foldFrench` produces — accents dropped, gender notices cut ("(H/F)", "F/H", "(e)", "·ne", ".e", "/euse"), feminine words folded to masculine ("développeuse", "chargée") — enforced by `TestFrenchAliasesAreFolded`. The main table is first asked again of the gender-cut title ("Ingénieur·e DevOps").

It reads only titles that `looksFrench` (gender notice, French accent, elided article, French function word, French-only head noun), since commercial/assistant/agent/boutique/maintenance are English too; "Stage" is no cue. Workplace words (bank, hotel, shop, ...) are in `frenchWorkplaceTable`, consulted only when the title names no technical work ("Tech Lead Java - Transport").

Two fixes reach the MAIN table: `unchefFrench` rewrites every non-kitchen "Chef de/d'/des X" to "cheffe" (partie, cuisine, rang, salle, bar… keep `chef`) — it was filing ~900 French titles under hospitality; and the electrician's "CFO" (courants forts) is rewritten on French electrical/building titles, so "Technicien de maintenance CFO/CFA" is not finance. The German `-monteur` gives way to the French video editor.

Bare "développeur" resolves here after business, property, territorial and network developers are vetoed. Field-less heads (technicien, chargé, responsable, conseiller, ingénieur, directeur, ...) resolve nothing.

Grades (`frenchSeniorityTable`): stage/stagiaire, alternance/alternant, apprenti intern (outrank lead); `confirmé` middle; `débutant`, `jeune diplômé` junior; `chef d'équipe`, `chef de groupe`, `responsable d'équipe` lead. "Chef de projet", responsable, manager, directeur, expert state no grade; only general/executive/financial directors and PDG are c_level. An executive's assistant and "Responsable alternance" are cut first.

Measured 2026-10-28 over 24,609 French postings (whatjobs excluded): category 20.5% → 72.8%, seniority 11.9% → 27.0%; 62 of 16,000 control titles changed, all correct.

## Polish stems

Polish INFLECTS (programista, programistka, programistów; genitive after "ds."), so `polish.go` matches STEMS on the title `foldPolish` produces (lowercase, diacritics dropped, "ł" → "l", gender forms cut, "Mł."/"St." spelled out): an alias ending in `*` matches any word starting with it (the opposite of the German head). Enforced by `TestPolishAliasesAreFolded`. The main table is first asked again of the gender-cut title ("Inżynier(-ka) Sieciowy(-a)").

It reads only titles that `looksPolish`: a Polish letter, a "(K/M)" notice (with "k" — "(m/f/d)" is German), a Polish-only word, a masculine/feminine pair, a "/-ka" suffix, or a case ending (`-owy/-owych` with "w"; Czech/Slovak use "v"). Bare "operator", "administrator", "architekt", "technik", "inspektor", "lider" are NOT cues (an English "$31/hr … Operator" went to hr through them).

Each answer follows the English twin (Relationship Manager finance, SAP Consultant solutions engineering, tokarz/frezer skilled trades). Role nouns outrank fields ("Elektroradiolog z obsługą recepcji"); "darmowy transport" is a perk, "branża produkcyjna" the client's sector. Generic heads (specjalista, kierownik, koordynator, doradca, inżynier, ...) resolve nothing; "Inżynier Systemów Hydraulicznych" is not "inżynier systemowy".

Grades (`polishSeniorityTable`): młodszy/Mł. junior (bare "ml" is machine learning), starszy/St. senior, "Regular" middle, stażysta/praktykant/staż/praktyki intern, kierownik/lider zespołu, an opening "Lider", wiodący lead, prezes and general/executive/financial directors c_level. "Specjalista", plain "kierownik", "dyrektor" state no grade; a lead's assistant/deputy and an internships desk are cut first.

Measured 2026-10-29 over 5,096 Polish postings: category 64.5% → 84.1%, seniority 4.4% → 11.2%; over 27,690 control titles 4 changed, all Polish. "Data Warehouse" now reads data engineering in the main table (was logistics).

## Croatian and Serbian heads and fields

`bcs.go` covers Croatian, Serbian, Bosnian and Montenegrin. The main table lists BCS nouns (#3797) only as nominatives, but the field after a head is genitive/locative/instrumental ("Voditelj trgovine", "Radnik u proizvodnji"), so an alias may end in `*`: a stem taking one of the closed endings in `bcsEndings`. Folding: Serbian Cyrillic transliterated (official mapping), gender forms cut ("(m/ž)", "Voditelj/ica", "INŽENJER/KA"), diacritics dropped, đ → "dj". The main table is asked again of that title.

Bands as in Spanish: open applications, scholarships and mystery shopper vetoed; head+field pairs first (business development sales, claims handler finance); role nouns (inflected and feminine); fields; desk words. Field-less heads (voditelj, specijalist, referent, tehničar, inženjer, ...) resolve nothing.

**Cyrillic** is transliterated only when the title is Serbian: a Serbian/Macedonian-only letter (ђ ј љ њ ћ џ) or a `serbianCyrillicCue` word (програмер, возач, продавац, конобар…). Without the gate, Russian/Ukrainian titles in shared letters ("Шеф-повар", "Контролер", "Кухар") changed category. A Latin title with a Cyrillic homoglyph ("CEO (СЕО)") is untouched.

Grades (`bcsSeniorityTable`): pripravnik, praksa, praktikant, stažist intern (outrank lead); mlađi, niži, početnik junior; viši/viša, stariji senior ("više" = "more", absent); vođa, voditelj tima/smjene, šef, vodeći lead — "vođa"/"šef" keep diacritics because Slovenian "vodja" is any manager. Only general/executive/financial directors and the management board are c_level; a lead's or executive's assistant/deputy is cut first.

Measured 2026-10-29 over 1,921 hr/rs/ba/me postings: category 63.2% → 79.1%, seniority 2.4% → 6.7%; over 45,088 control titles no Russian/Ukrainian title changed, the 28 that changed are correct. Talentlyft titles stored with HTML entities ("Prodava&#x10D;") stay unresolved.

## Slovenian grades and fields

Slovenian titles reach the BCS machinery, but key words differ (senior "višji" not "viši", junior "mlajši", shift lead "vodja izmene", machine operator "posluževalec"/"upravljalec"). `slovenian.go` is read after the BCS table resolved nothing. Nouns with a fleeting "e" ("posluževalec", gen. "posluževalca") are listed in both stems.

The CEE block already resolved 88% (2026-10-29, 2,900 MojeDelo + 3,621 ESS); the rest are mostly field-less heads ("Svetovalec", "Referent"), left unresolved like their English twins. "na CSD" is social work; bare "CSD" is also a hospital sterile department. "Poslovodja" is store manager; BCS "poslovođa" (also a site foreman) and "Projektni inženir" stay unresolved.

Grades: višji/višja/višjega senior — never "višje"/"višjo" ("higher", "z višjo izobrazbo"); mlajši, začetnik junior; študentsko delo, pripravništvo intern; vodja izmene/skupine/ekipe, izmenovodja, skupinovodja, "vodilni" engineer/developer lead; finančni direktor c_level. Bare "vodja" and plain "direktor" state no grade. A lead's assistant/deputy ("Pomočnik vodje", "Namestnik vodje", "Pomočnik generalnega direktorja") is cut first. Measured: MojeDelo category 88.4% → 90.8%, seniority 1.4% → 4.6%; ESS 88.1% → 90.9%, 4.2% → 6.2%; every change hand-checked.

`classify.IsTech` false for "PHP backend programer" is intended: programmer nouns stay out of the title-term detector (a term there wins outright; "CNC operater - programer" is a machine operator). Such a posting is technical via its category in `jobderive.deriveIsTech`.

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
