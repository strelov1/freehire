## ADDED Requirements

### Requirement: Marketing disciplines resolve to granular named roles

The role dictionary SHALL resolve the marketing disciplines that the coarse
`marketing` category flattens into distinct named role slugs, so a candidate can
filter to one discipline rather than to all of marketing. It SHALL cover four
clusters: search optimization (technical SEO, content SEO, link building, SEO
analysis), generative-engine optimization, social media (social media management,
community management, paid social, content creation), and the commercial marketing
functions (product marketing, growth, demand generation, lifecycle, performance,
marketing operations, CRM/email, brand, PR, influencer, copywriting, marketing
analysis). Each slug SHALL carry a human label in the role catalog, under the
existing named-role rules — longest alias wins, whole-word match, never guesses.

#### Scenario: A technical SEO title resolves to its own role

- **WHEN** roles are derived for a job titled "Technical SEO Specialist"
- **THEN** the derived roles include `technical_seo_specialist`, distinct from the
  role a job titled "SEO Content Writer" derives

#### Scenario: Each discipline cluster is separable

- **WHEN** roles are derived for jobs titled "Community Manager", "Paid Social
  Specialist", "Demand Generation Manager" and "Lifecycle Marketing Manager"
- **THEN** each derives its own distinct role slug, and no two of them collapse to
  the same slug

#### Scenario: Every new role has a catalog label

- **WHEN** the role catalog is read for any marketing role slug the dictionary can derive
- **THEN** that slug maps to a human label

#### Scenario: An unrecognized marketing title still yields nothing

- **WHEN** roles are derived for a marketing title no alias covers
- **THEN** no marketing named role is emitted for it

### Requirement: Granularity comes from roles, not from new categories

The marketing disciplines SHALL remain under the existing `marketing` category:
this change SHALL NOT add a member to the category vocabulary, and every marketing
title alias added to the category dictionary SHALL resolve to `marketing`. The one
exception is the GTM-engineering title family, which SHALL resolve to `sales`,
where revenue-operations titles already sit. Consequently the tri-state `is_tech`
derivation and the non-technical category set SHALL be unchanged by this change.

#### Scenario: A new marketing alias resolves to the existing category

- **WHEN** a title carrying a newly added marketing alias (e.g. "Demand Generation
  Manager") is classified
- **THEN** its category is `marketing`

#### Scenario: GTM engineering resolves to sales

- **WHEN** a job titled "GTM Engineer" or "Go-To-Market Engineer" is classified
- **THEN** its category is `sales`, and the derived roles include the GTM
  engineering role slug

#### Scenario: The category vocabulary is unchanged

- **WHEN** the category vocabulary is enumerated after this change
- **THEN** it contains exactly the members it contained before, and the
  non-technical category set is likewise unchanged

### Requirement: Marketing title aliases MUST NOT capture technical roles

Every marketing title alias added to the category dictionary SHALL be specific
enough that it cannot claim a technical role. A bare discipline noun that also
occurs inside technical titles ("growth", "content", "brand", "performance") SHALL
NOT be added as a standalone alias — only as part of a phrase that names the
marketing role. A technical title that merely contains a marketing word SHALL keep
the category and the `is_tech` value it resolved before this change.

#### Scenario: A growth-engineering title stays technical

- **WHEN** a job titled "Growth Engineer" is classified
- **THEN** its category is not `marketing`, and it retains the role slug and
  `is_tech` value it had before this change

#### Scenario: A content-platform engineering title stays technical

- **WHEN** a job titled "Content Platform Engineer" is classified
- **THEN** its category is not `marketing`

#### Scenario: The phrase form still resolves

- **WHEN** a job titled "Growth Marketing Manager" is classified
- **THEN** its category is `marketing`

### Requirement: GTM means go-to-market, and the tag manager must be spelled out

`GTM` names Go-To-Market in a job posting far more often than it names Google Tag
Manager: "GTM strategy", "GTM motion" and "GTM Engineer" are the common forms,
while the tag manager is normally written out or named as a container. The
abbreviation SHALL therefore resolve to the go-to-market meaning, and the tag
manager SHALL resolve only from an unambiguous phrase — the spelled-out product
name or a container-scoped form. The abbreviation SHALL NOT resolve to
`google-tag-manager` in any case form.

#### Scenario: The abbreviation names go-to-market

- **WHEN** a job's text says "own our GTM strategy" or "shape the GTM motion"
- **THEN** the emitted skills include the go-to-market canonical and do NOT
  include `google-tag-manager`

#### Scenario: The title phrase names the go-to-market role

- **WHEN** a job titled "GTM Engineer" is processed
- **THEN** the derived roles include the GTM engineering role

#### Scenario: The tag manager needs its spelled-out name

- **WHEN** a job's text mentions "Google Tag Manager" or a "GTM container"
- **THEN** the emitted skills include `google-tag-manager`

#### Scenario: A bare abbreviation among tools does not name the tag manager

- **WHEN** a job's text lists "GTM" alongside GA4 with no spelled-out form
- **THEN** no `google-tag-manager` skill is emitted

### Requirement: The GEO homonym never leaks into geography

`GEO` names Generative Engine Optimization in a marketing title and geography
everywhere else. The role dictionary SHALL resolve the generative-engine-optimization
role only from a phrase that disambiguates it — the spelled-out form, the
`AEO`/answer-engine variants, or `GEO` bound to a search-marketing word — and SHALL
NOT resolve it from the bare `geo` token. Likewise the skill dictionary SHALL NOT
resolve a bare `geo` token to the optimization discipline.

#### Scenario: The spelled-out form resolves

- **WHEN** roles are derived for a job titled "Generative Engine Optimization
  Specialist"
- **THEN** the derived roles include the GEO role

#### Scenario: The bound abbreviation resolves

- **WHEN** roles are derived for a job titled "SEO / GEO Specialist" or "GEO
  Specialist"
- **THEN** the derived roles include the GEO role

#### Scenario: A geospatial title is untouched

- **WHEN** roles are derived for a job titled "Geo Data Analyst" or "Geospatial
  Engineer"
- **THEN** no GEO optimization role is emitted

#### Scenario: The answer-engine variant is the same role

- **WHEN** roles are derived for a job titled "Answer Engine Optimization Manager"
- **THEN** the derived roles include the same GEO role slug as the spelled-out form

### Requirement: Marketing tooling resolves as skills

The skill dictionary SHALL resolve the platforms and disciplines that marketing
postings name, so a discipline filter can be combined with a tool filter. Coverage
SHALL include search tooling (Semrush, Ahrefs, Screaming Frog, Google Search
Console), lifecycle and email platforms (Klaviyo, Mailchimp, Customer.io),
advertising platforms (Google Ads, Meta Ads, TikTok Ads, LinkedIn Ads),
measurement and tagging (Google Tag Manager, Looker Studio, Amplitude, Mixpanel),
social management tooling (Hootsuite, Sprout Social), and the disciplines
themselves (technical SEO, link building, paid social,
demand generation, lifecycle marketing, marketing automation, generative engine
optimization, content marketing, email marketing, influencer marketing,
copywriting, PPC). Each SHALL resolve under the existing curated-only rules and
SHALL emit nothing for a term it does not know. A product whose name is an
ordinary English word in marketing prose — Segment, Buffer, Later — SHALL be
excluded rather than gated: "the customer segment" and "a content buffer" occur in
exactly the postings this coverage serves.

#### Scenario: A search tool resolves to its canonical

- **WHEN** a job's text mentions "Ahrefs", "Semrush" or "Screaming Frog"
- **THEN** the emitted skills include the corresponding canonical

#### Scenario: A discipline resolves as a skill

- **WHEN** a job's text mentions "link building" or "demand generation"
- **THEN** the emitted skills include the corresponding canonical

#### Scenario: Separator forms resolve alike

- **WHEN** a job's text mentions "paid-social", "paid social" or "paid_social"
- **THEN** each resolves to the same canonical

#### Scenario: A tool named after an ordinary word does not tag

- **WHEN** a posting says "define the customer segment" or "keep a content buffer"
- **THEN** no skill canonical is emitted for the colliding word

#### Scenario: An ad platform is not confused with the vendor

- **WHEN** a job's text mentions "Google Ads"
- **THEN** the emitted skills include the Google Ads canonical, and the mention
  alone does not emit an unrelated Google-product canonical
