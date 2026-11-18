# internal/dict/classify — the Romance and Slavic language tables

Part of [AGENT-REFERENCE.md](AGENT-REFERENCE.md): sections moved out of it for size.

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

## Turkish IT titles

Turkish is agglutinative ("Uzman", "Uzmanı"), so `turkish.go` matches STEMS on the title `foldTurkish` produces (diacritics and the dotless ı dropped, and the combining dot that lowercasing "İ" leaves behind), reusing the Polish word splitter and phrase matcher; a hyphen separates words. An alias opening with `^` must start the title ("Sistem Uzmanı" is the system specialist, "Kalite Sistem Uzmanı" is not). It is the last category fallback and the last grade fallback, and reads only a title with a Turkish role word (`turkishCues`), so a bare `sap`/`erp` cannot reach an English title.

Scope is the IT vocabulary only, measured on the 437 titles of Kariyer.net's IT work areas (2026-11-17): `IsTech` 30 → 190, category resolved 60 → 284, either 62 → 310. Each answer follows the English twin (Bilgi İşlem Uzmanı = IT Specialist is it_support, İş Analisti business_analysis, SAP/ERP Uzmanı solutions_engineering, Siber Güvenlik security). Where the twin is doubtful the title stays unresolved: the English "IT Manager" is devops, so "Bilgi İşlem/Teknolojileri Müdürü" is vetoed with `categoryNone` (still technical via `IsTech`); "Sistem Mühendisi", "Veri Tabanı Uzmanı", "Yapay Zeka Uzmanı", "Teknik Destek Uzmanı" and "Test Uzmanı" resolve nothing, as bare "AI Specialist" and "Technical Support" would not be read as IT either. `turkishTechTerms` is anchored to the discipline like `techTitleTerms`: bare "yazılım" is not a term ("Yazılım Satış Uzmanı" is sales).

Grades: Kıdemli senior, Stajyer intern, "Uzman Yardımcısı" junior; bare "Uzman" states none.

Beyond IT (`turkish_general.go`, read after the IT table): the same stems, folded title and `^` anchor, for every other profession. Measured on Kariyer.net's whole catalogue (24,956 postings, 2026-11-17, read one work area at a time), a category resolves for 70.9% of the titles, against 6.7% before. The table is banded and its order is the precedence: vetoes and roles whose field word would file them elsewhere (a medical representative and an insurance or real-estate consultant sell; a "Satış Mühendisi" is left unresolved, its English twin being a technical category and the Turkish ones sellers at a plant), then the store and the sales desk, which outrank every other word ("Temizlik Ürünleri Satış Temsilcisi" sells, "Mağaza Depo Sorumlusu" works in the store), then the role nouns (garson, hemşire, şoför, kasiyer), then the fields (muhasebe, üretim, kalite), then the engineers by discipline. "Güvenlik Görevlisi" is the guard (`public_safety`); bare "güvenlik" is information security and is read by the IT table first. A generic head (Uzman, Danışman, Sorumlu, Müdür, Yönetici, Eleman, Personel) carries no category of its own, and what the English twin leaves unresolved stays so: "Müşteri İlişkileri Uzmanı", "Planlama Uzmanı", a bare "Mimar" (the building and the software architect share it) and the "Genel Başvuru" general applications. The cues that let a title in are Turkish role words only; "market" and "operator" are English words too and are not cues. The 29% still unresolved are mostly those generic heads, the general applications and "Satış Mühendisi".
