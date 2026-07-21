## ADDED Requirements

### Requirement: The industry-domain vocabulary is canonical and defined

The system SHALL define a single canonical industry-domain vocabulary describing the **industry/vertical of the company or product** behind a job (what the company does), distinct from the job's role category. Every domain value SHALL carry a one-line definition. A domain value SHALL name a vertical, never a business model.

The vocabulary SHALL be: `fintech`, `crypto`, `ecommerce`, `gambling`, `gamedev`, `media`, `travel`, `healthcare`, `edtech`, `govtech`, `devtools`, `cybersecurity`, `ai`, `hrtech`, `adtech`, `proptech`, `logistics`, `mobility`, `climatetech`, and the residual `other`.

#### Scenario: Every domain has a definition

- **WHEN** the domain vocabulary is enumerated
- **THEN** each value carries a one-line definition and names a vertical (industry), not a delivery model

### Requirement: saas is removed as a domain

The vocabulary SHALL NOT contain `saas`. `saas` describes a business model (subscription delivery) that overlaps every vertical and does not tell a seeker what the company does. Its coverage SHALL be replaced by `devtools` for horizontal developer/infra products and by the appropriate functional vertical (e.g. `hrtech`, `adtech`, `cybersecurity`); generic horizontal SaaS with no vertical SHALL resolve to `other`.

**Reason**: `saas` is a delivery model, not an industry vertical; it structurally overlaps all other domains and degrades filter precision.
**Migration**: Existing rows carrying `saas` are corrected by re-enrichment; because `domains` is a validated served facet, any `saas` value is dropped from served output once removed from the vocabulary, so no schema/index change is required.

#### Scenario: saas is not an accepted value

- **WHEN** an enrichment payload proposes `saas` as a domain
- **THEN** the value is not in the vocabulary and is dropped from the served facet

### Requirement: Overlapping verticals fold into a single canonical value

The vocabulary SHALL collapse synonym/sub-vertical labels into one canonical value rather than admitting parallel near-duplicates: `web3`/`defi`/`nft` → `crypto`; `insurtech`/`regtech`/`wealthtech` → `fintech`; `martech` → `adtech`; `social`/`socialmedia`/`dating`/creator-economy → `media`; `biotech`/`medtech`/`healthtech` → `healthcare`; `retail`/`retailtech` → `ecommerce`; `greentech`/`cleantech`/`energy` → `climatetech`. The `crypto`/`fintech` boundary SHALL be: a product built on a blockchain or issuing/trading tokens is `crypto`; a product moving money over traditional financial rails is `fintech`; a product doing both carries both.

#### Scenario: A web3 product is classified as crypto

- **WHEN** a company builds a decentralized exchange
- **THEN** its domain is `crypto`, and no separate `web3` value exists

### Requirement: The domain vocabulary is glossed for the enrichment LLM

The enrichment prompt SHALL present each domain value with its one-line definition (not as a bare name list), so the LLM classifies on what the company does. The `ai` gloss SHALL scope it to a company whose **core product is AI/ML** (model providers, AI/ML platforms, applied-AI products), explicitly excluding companies that merely use AI, so it does not swallow the majority of tech companies.

#### Scenario: The prompt carries per-value definitions

- **WHEN** the enrichment prompt is assembled for the `domains` field
- **THEN** each domain value appears with its one-line definition, and `ai` is glossed as core-product-AI only

#### Scenario: A company that merely uses AI is not tagged ai

- **WHEN** a fintech company that uses ML for fraud detection is enriched
- **THEN** its domain is `fintech` (its core product), not `ai`
