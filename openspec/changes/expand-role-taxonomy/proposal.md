## Why

freehire aims to be an **IT/tech** job catalogue, but its role-category dictionary only names engineering + a handful of adjacent roles (design/product/PM) plus four business functions (marketing/sales/support/management). Every other role that exists inside a tech company — recruiter, HR, finance, legal, ops, customer success, solutions engineer, tech writer, business analyst — resolves to `other`/unknown, so it cannot be filtered, counted, or cleanly separated from the genuinely non-IT tail (cook, nurse, driver). The industry-`domain` vocabulary has the mirror problem: it carries a non-vertical (`saas`), lacks high-volume verticals (devtools, cybersecurity, AI, hrtech), and is handed to the LLM as a bare word-list with no definitions, which drives misclassification.

This change defines the **full** role-category and industry-domain vocabularies explicitly and expands them to cover all IT-company roles, so the catalogue can present itself as a focused IT product (the complementary source-curation step is tracked separately).

## What Changes

- Add **10 new role categories** covering the IT-company roles that currently have no home: `recruiting`, `hr`, `finance`, `legal`, `operations`, `customer_success`, `business_analysis`, `solutions_engineering`, `developer_relations`, `technical_writing`. Each ships curated EN+RU title aliases and a skill set (grounded in ESCO/O*NET/BABOK/industry sources).
- **Split `customer_success` out of `support`** — post-sale success/renewals is a distinct function from reactive helpdesk. **BREAKING** for the `support` facet's membership.
- Route **BI titles** (`bi analyst`, `business intelligence …`) into the existing `data_analytics`; route **RevOps/Sales Ops** into the existing `sales`.
- Fix the **alias-ordering hazard**: the terminal fall-throughs `analyst→data_analytics` and `manager→management` silently steal `financial analyst`, `business analyst`, `operations manager`, etc. Every new `…analyst`/`…manager`/`…engineer` alias is placed above them; `sales engineer` above bare `sales`; `ux writer`/`content designer` above `ux`/`designer`. `csm` is deliberately NOT added (collides with Certified Scrum Master).
- **Partition the new categories** into the existing tech/business split: the four IT-product-adjacent roles (`business_analysis`, `solutions_engineering`, `developer_relations`, `technical_writing`) join `TechCategories` (consistent with `design`/`product`/`project_management` already being tech, and so they are LLM-enriched); the six back-office roles (`recruiting`, `hr`, `finance`, `legal`, `operations`, `customer_success`) join `NonTechCategories` (so `is_tech=false` and they stay out of the LLM enrich budget, like `marketing`/`sales`).
- Revise the **industry-domain** vocabulary: **drop** `saas` (a business model, not a vertical); **add** `devtools`, `cybersecurity`, `ai`, `hrtech`, `proptech`, `climatetech`, `mobility`; fold synonyms (web3→crypto, insurtech/regtech→fintech, martech→adtech, social/dating→media, biotech→healthcare, retail→ecommerce). Ship a **one-line definition per domain into the LLM prompt**.
- **Re-derive existing rows**: run `cmd/backfill-derive` so the expanded category/skill dictionaries re-classify the whole catalogue, then reindex. (Domains are LLM-only and are not touched by backfill — historical domain values are corrected only by re-enrichment.)

## Capabilities

### New Capabilities
- `role-category-taxonomy`: the canonical, fully-defined role-category vocabulary — every category (existing + new) with a one-line definition, its tech/business partition membership, and the alias-ordering doctrine that keeps specific role titles from being stolen by the terminal fall-throughs.
- `industry-domain-taxonomy`: the canonical industry-domain vocabulary — every domain value with a one-line definition and the per-value gloss supplied to the enrichment LLM, plus the fold/rename/drop rules against the previous list.

### Modified Capabilities
<!-- None. The affected existing capabilities (tech-classification, skill-tag-matching,
     deterministic-facets, job-enrichment, ai-enrichment) are parameterized over the
     vocabularies this change defines: is_tech is stated over "recognized technical
     category", the unified backfill already re-derives `category`, and skill-tag matching
     already resolves "only known aliases". Their normative requirements do not change —
     only the vocabulary + dictionary DATA they operate on, which the two new capability
     specs own. The code touched is listed under Impact. -->
- (none — no existing requirement's normative behavior changes)

## Impact

- **Code**: `internal/classify/dictionaries.go` (categoryTable aliases + ordering), `internal/classify/tech.go`/`nontech.go` (only if title detectors need new anchored terms), `internal/enrich/enrichment.go` (`CategoryValues`, `TechCategories`, `NonTechCategories`, `DomainValues`), `internal/enrich/langchain.go` (domain prompt glosses), `internal/skilltag` (new role skills), `internal/enrich/techcategories_test.go` (partition test must stay green with the new members).
- **Frontend**: `web/src/lib/labels.ts` (`CATEGORY_LABELS`, `DOMAIN_LABELS`), regenerated `web/src/lib/generated/contracts.ts`; category/domain filter option lists.
- **Data / ops**: `go run ./cmd/backfill-derive` then `make reindex` to re-classify + re-index existing rows; decide the `saas`-value migration for historical domain rows (remap or re-enrich).
- **Not in scope**: dropping non-IT industry *sources* (the complementary "focus" lever) is a separate change; `is_tech` remains an honest tech/non-tech signal here rather than being repurposed as an in-scope flag.
