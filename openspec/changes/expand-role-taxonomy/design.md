## Context

`internal/classify/dictionaries.go` holds `categoryTable`, an ordered alias→canonical list matched whole-word against the job **title**; `internal/enrich/enrichment.go` holds the `CategoryValues` vocabulary and its `TechCategories`/`NonTechCategories` partition (a test asserts they partition exactly), plus the LLM-only `DomainValues`. Category derivation is deterministic and wins over the LLM; domains are emitted by the LLM from a bare name list. The existing partition already treats `design`/`product`/`project_management` as **technical** ("IT-industry roles"), so the tech/non-tech split is really "core IT/product role vs business back-office", not "engineer vs not". Aliases and skills below are grounded in ESCO, O*NET, IIBA/BABOK, YC/Crunchbase verticals and industry references (full source list in the change discussion).

## Goals / Non-Goals

**Goals:**
- Define the full role-category and industry-domain vocabularies with per-value meaning.
- Add the 10 IT-company role categories with curated EN+RU title aliases and skills.
- Keep the single tech/business partition serving both `is_tech` and the enrich cost-gate.
- Revise domains (drop the model `saas`, add real verticals, gloss the prompt).
- Re-derive existing rows deterministically.

**Non-Goals:**
- Dropping non-IT industry *sources* (the complementary focus lever) — separate change.
- Repurposing `is_tech` as an in-scope flag — it stays an honest tech/business signal.
- Backfilling historical `domains` beyond re-enrichment (domains are LLM-only).

## Decisions

### D1 — Partition placement doubles as the enrich-gate (no decoupling)
Place the four IT-product-adjacent roles (`business_analysis`, `solutions_engineering`, `developer_relations`, `technical_writing`) in `TechCategories` and the six back-office roles (`recruiting`, `hr`, `finance`, `legal`, `operations`, `customer_success`) in `NonTechCategories`. This is consistent with `design`/`product`/`project_management` already being tech, gives an honest `is_tech` (a recruiter is not tech), and — because `NonTechCategories` also gates LLM enrichment off — keeps the six business roles out of the enrich budget while the four tech-adjacent roles get enriched. One list, two uses, no new decoupling needed. *Alternative considered:* a separate `is_in_scope` column — rejected as over-engineering for a flag that becomes redundant once sources are IT-curated.

### D2 — Ordering is load-bearing
`categoryTable` is precedence-ordered. All new blocks go **above** the terminal fall-throughs `{"analyst","data_analytics"}`, `{"manager","management"}`, `{"1c","backend"}`; `sales engineer` above `{"sales","sales"}`; `ux writer`/`content designer` above `{"ux",…}`/`{"designer",…}`; functional-ops (`sales operations`/`marketing operations`) inside their own sales/marketing blocks so they never leak into `operations`. `csm` is never added (Certified Scrum Master collision).

### D3 — Fold, don't proliferate
BI → existing `data_analytics`; RevOps/Sales Ops → existing `sales`; `solutions architect` stays in `architecture` (documented ambiguity, no regression). Domain synonyms fold into one canonical (web3→crypto, insurtech→fintech, martech→adtech, etc.).

### D4 — Gloss the domain prompt
The single highest-precision change for domains is giving the LLM a one-line definition per value (today it gets a bare list). `ai` is scoped to core-product-AI only.

## Role-category aliases (title → canonical), EN + RU

Confident whole-word aliases only; place per D2. Condensed from the research; full skill lists follow.

**`recruiting`** — talent acquisition / recruiter / sourcer (candidate pipeline).
`recruiter, tech recruiter, technical recruiter, it recruiter, talent acquisition, talent acquisition specialist/partner/manager, talent sourcer, sourcing specialist, recruitment consultant/specialist, recruiting manager/coordinator, head of talent, head of talent acquisition` · RU `рекрутер, рекрутёр, ит-рекрутер, специалист/менеджер по подбору персонала, ресечер, ресёчер, специалист по найму`

**`hr`** — generalist people function (HRBP, people ops, L&D, C&B, HR leadership).
`human resources, hr manager/generalist/specialist, hr business partner, hrbp, people partner, people operations, people ops, hr director, head of people, vp people, chief people officer, chief human resources officer, chro, learning and development, l&d specialist, compensation and benefits, c&b specialist` · RU `hr-менеджер, менеджер/специалист/директор по персоналу, специалист по компенсациям и льготам, специалист по обучению и развитию, эйчар`

**`finance`** — accounting/treasury/FP&A/finance leadership.
`chief financial officer, cfo, vp finance, head of finance, financial/corporate/finance controller, financial analyst, finance analyst, fp&a, financial planning, accountant, accounting, bookkeeper, bookkeeping, payroll, treasury, treasurer, tax accountant, financial reporting, finance manager, financial manager` · RU `финансовый директор, главный бухгалтер, главбух, бухгалтер, финансовый аналитик/менеджер/контролёр/контролер, казначей, аудитор` · *exclude bare `controller` (PLC/game-controller collision).*

**`legal`** — counsel/paralegal/compliance/privacy.
`general/legal/corporate/associate general/privacy counsel, legal manager/assistant/operations, lawyer, attorney, paralegal, contract manager/administrator, contracts manager, compliance officer/manager/analyst/specialist, regulatory affairs, data protection officer` · RU `юрист, юрисконсульт, корпоративный юрист, помощник юриста, комплаенс, комплаенс-менеджер, специалист по комплаенсу` · *anchor `compliance …` (never bare `compliance`).*

**`operations`** — bizops/ops-mgmt/office/procurement/chief of staff.
`chief operating officer, coo, chief of staff, business operations, biz ops, operations manager/analyst/associate/lead, ops manager, head of operations, office manager, executive assistant, administrative assistant, procurement, procurement/purchasing manager, facilities manager, workplace manager` · RU `операционный директор/менеджер, офис-менеджер, ассистент/помощник руководителя, личный помощник, специалист по закупкам, закупщик, менеджер по закупкам` · *never bare `operations`/`ops` (DevOps/SecOps/MLOps collision).*

**`customer_success`** — post-sale success/onboarding/renewals.
`customer success (manager/specialist/engineer/associate/operations), client success (manager), customer onboarding, onboarding specialist/manager, implementation specialist/manager/consultant, renewals/renewal manager, customer retention` · RU `менеджер по успеху клиентов, менеджер по работе с клиентами, менеджер по сопровождению клиентов, специалист по адаптации клиентов` · *never bare `csm`; keep `account manager`→`sales`.*

**`business_analysis`** — requirements/process/systems analysis (BABOK).
`business analyst, business systems analyst, business system analyst, systems analyst, system analyst, business process analyst, process analyst, requirements analyst, functional analyst, it business analyst, solutions analyst, erp analyst, business analysis` · RU `бизнес-аналитик, бизнес аналитик, системный аналитик, системный бизнес-аналитик, аналитик требований, аналитик бизнес-процессов, функциональный аналитик, бизнес-анализ` · *never bare `analyst`; `product analyst`→`data_analytics` (flagged, human call).* Also add to `data_analytics`: `business intelligence analyst, bi analyst, business intelligence developer, bi developer` · RU `аналитик bi, bi-аналитик, аналитик business intelligence`.

**`solutions_engineering`** — technical pre-sales/field.
`solutions/solution engineer, sales engineer, presales/pre-sales/pre sales engineer, solutions/solution consultant, presales consultant, sales applications engineer, field engineer, forward deployed engineer, customer engineer` · RU `пресейл, пресейл-инженер, пресейл инженер, инженер пресейл, технический пресейл` · *`sales engineer` above bare `sales`; `solutions architect` stays `architecture`.*

**`developer_relations`** — DevRel/advocacy/evangelism.
`developer advocate, developer relations, devrel, developer evangelist, technical evangelist, developer experience engineer, community engineer, developer community manager` · RU `деврел, девелопер адвокат, технический евангелист` · *anchor `developer/technical evangelist` (never bare `evangelist`).*

**`technical_writing`** — docs/API/UX writing/localization.
`technical writer, technical writing, technical communicator, documentation specialist/manager/engineer, information developer, content designer, ux writer, content strategist, localization specialist/manager/engineer` · RU `технический писатель, техписатель, технический редактор, разработчик документации, писатель технической документации, специалист по документации, ux-редактор` · *`ux writer`/`content designer` above `ux`/`designer`; keep `copywriter`/`content writer`→`marketing`.*

## Role-category skills (for `skilltag`, condensed; full sourced lists in research)

- **recruiting**: greenhouse, lever, workday recruiting, icims, smartrecruiters, ashby, jobvite, linkedin recruiter, boolean search, x-ray search, gem, hireez, seekout, full-cycle recruiting, talent sourcing, candidate screening, technical screening, offer negotiation, candidate experience, employer branding, diversity recruiting, time-to-fill, cost-per-hire, quality of hire.
- **hr**: hris, workday, sap successfactors, adp workforce now, bamboohr, paycom, personio, payroll, onboarding, offboarding, employee relations, performance management, talent management, succession planning, org design, compensation and benefits, benefits administration, lms, people analytics, employee engagement, labor law, gdpr, dei, shrm-cp, phr.
- **finance**: quickbooks, netsuite, sap, oracle financials, xero, sage intacct, workday financial management, adaptive planning, planful, coupa, bill.com, ramp, brex, excel, financial modeling, fp&a, gaap, ifrs, revenue recognition, accounts payable/receivable, general ledger, reconciliation, budgeting, forecasting, payroll, adp, gusto, treasury management, financial reporting, tax, audit.
- **legal**: contract lifecycle management, clm, ironclad, docusign clm, juro, contract drafting/negotiation/review, redlining, corporate law, intellectual property, nda, due diligence, e-discovery, legal research, gdpr, ccpa, data privacy, regulatory compliance, aml, kyc, sox, soc 2, regulatory affairs.
- **operations**: process improvement, operational efficiency, program management, procurement, vendor management, spend management, coupa, okrs, kpis, strategic planning, cross-functional coordination, stakeholder management, sql, excel, salesforce, notion, asana, jira, workflow automation, facilities management, event planning, calendar management, six sigma, lean.
- **customer_success**: customer onboarding, product adoption, customer retention, churn prevention, renewals, upsell, cross-sell, expansion revenue, qbr, customer health score, success planning, escalation management, customer advocacy, gainsight, churnzero, totango, pendo, salesforce, hubspot, crm, nps, csat, forecasting.
- **business_analysis**: requirements elicitation/gathering/analysis, requirements traceability, babok, stakeholder management, bpmn, process modeling/mapping, use cases, user stories, uml, data modeling, erd, functional specifications, gap analysis, workflow analysis, feasibility analysis, solution evaluation, acceptance criteria, sdlc, agile, scrum, jira, confluence, visio, sql.
- **solutions_engineering**: pre-sales, sales engineering, product demonstration, proof of concept, technical discovery, rfp response, rfi response, technical presentations, solution design, product configuration, api integration, salesforce, crm, sql, python, javascript, aws, azure, gcp, saas, troubleshooting, technical documentation, integration, scripting.
- **developer_relations**: developer advocacy, technical evangelism, technical content creation, public speaking, conference talks, community management, developer experience, api, sdk, open source, tutorials, sample code, technical writing, blogging, video content, storytelling, hackathons, meetups, python, javascript, go, documentation.
- **technical_writing**: technical writing, documentation, api documentation, docs-as-code, markdown, dita, xml, madcap flare, adobe framemaker, confluence, git, docusaurus, openapi, swagger, structured authoring, information architecture, content strategy, ux writing, editing, style guide, user guides, release notes, readme, localization, translation.

## Industry-domain final vocabulary + prompt gloss

Drop `saas`. Keep 13, add 6 (+`mobility`). Ship this gloss into `internal/enrich/langchain.go`:

```
fintech       payments, banking, lending, wealth/trading, insurtech, regtech (traditional financial rails)
crypto        blockchain, web3, DeFi, tokens/NFTs, exchanges, on-chain infra
ecommerce     online retail, marketplaces, D2C, retail/checkout/fulfillment tech
gambling      betting, casino/iGaming, sportsbook, lottery
gamedev       video-game development, publishing, game engines/infra
media         content, publishing, streaming, entertainment, social networks, dating, creator economy
travel        travel, hospitality, tourism, booking
healthcare    health-tech, medtech, digital health, biotech, pharma, wellness
edtech        education, e-learning, training, LMS
govtech       government, public sector, civic tech
devtools      developer tools, cloud infra, databases, DevOps, APIs, IT infrastructure
cybersecurity security software, identity, threat detection, appsec, privacy, fraud
ai            company whose CORE PRODUCT is AI/ML (model providers, AI/ML platforms, applied-AI) — NOT merely "uses AI"
hrtech        recruiting, HR, payroll, people-ops, staffing software
adtech        advertising and marketing technology (ad serving, attribution, CRM, marketing automation)
proptech      real-estate and construction technology
logistics     supply chain, freight, delivery, fleet, warehousing (goods)
mobility      automotive, autonomous vehicles, ride-hailing, transport of people
climatetech   climate, clean/renewable energy, sustainability
other         none of the above (incl. generic horizontal productivity/CRM SaaS)
```

## Risks / Trade-offs

- **Industry-ambiguous titles** (accountant, office manager, paralegal, recruiter) exist outside tech → will now categorize non-IT postings too. **Mitigation:** acceptable pre-source-curation; these are role facets, not is_tech signals; the focus lever is the separate source-curation change.
- **Category count 25 → 35** widens LLM/filter surface. **Mitigation:** folding (BI/RevOps/synonyms) and skipping thin verticals keeps growth bounded; `other` still absorbs the rest.
- **Alias ordering regressions** could silently mis-route. **Mitigation:** scenario tests for each fall-through (analyst/manager/sales/ux); run the existing classify test suite.
- **Dropping `saas`** strands historical rows. **Mitigation:** `keepKnown`/`Sanitize` drop it from served output automatically; re-enrich to repopulate; optional one-off remap `saas`→`devtools`/`other`.

## Migration Plan

1. Extend `categoryTable` (respect D2 ordering) + `classify` title detectors if needed.
2. Add the 10 values to `CategoryValues`; place 4 in `TechCategories`, 6 in `NonTechCategories`; keep the partition test green.
3. Add new-role skills to `internal/skilltag`.
4. Domains: edit `DomainValues` (drop `saas`, add 7), `web/src/lib/labels.ts` `DOMAIN_LABELS`, and the prompt gloss in `langchain.go` (the 3-edit enrichment convention). Regenerate contracts.
5. Add `CATEGORY_LABELS` for the 10 new categories; regenerate `web/src/lib/generated/contracts.ts`.
6. `go run ./cmd/backfill-derive` → re-derive category/skills on all rows; then `make reindex`.
7. Domains only: trigger re-enrichment for historical accuracy (backfill does not touch LLM facets).
8. **Rollback:** revert the vocabulary/dictionary edits and re-run backfill-derive + reindex; served facets converge back.

## Open Questions

- `product analyst` / `product operations` placement (flagged `data_analytics` / `business_analysis` respectively) — confirm or leave in `other`.
- `mobility` domain — ship now or defer until volume shows a cluster.
- `saas` historical rows — silent drop vs one-off remap to `devtools`/`other`.
- Whether `technical_writing` and `developer_relations` should be tech (enriched) — current call: yes, per D1.
