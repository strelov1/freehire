## Why

The catalogue flattens every marketing job into one coarse `marketing` category —
13 title aliases covering ~18k open vacancies. A candidate who does technical SEO
cannot separate it from copywriting, and the disciplines that emerged since the
dictionary was written (GEO/AEO — optimizing for generative answer engines — and
GTM engineering) resolve to nothing at all. The skill dictionary compounds it:
`seo` and `hubspot` resolve, but `semrush`, `ahrefs`, `screaming frog`, `google
search console`, `klaviyo`, `google ads`, `meta ads` and `google tag manager` —
the tools these postings actually name — resolve to nothing, so the "SEO role that
uses Ahrefs" filter cannot be expressed.

## What Changes

- **Granular marketing roles in `internal/roletag`.** ~20 named roles across four
  clusters the coarse category flattens: SEO (technical / content / link building
  / analyst), GEO-AEO, SMM (social media, community, paid social, content creator),
  and the GTM-adjacent commercial roles (GTM engineer, demand generation, lifecycle,
  performance marketing, marketing operations, CRM marketing, brand, PR, influencer,
  email, copywriter, marketing analyst).
- **Marketing tooling and disciplines in `internal/skilltag`.** ~30 canonicals:
  SEO/GEO tooling (semrush, ahrefs, screaming frog, google search console, moz),
  lifecycle and email platforms (klaviyo, mailchimp, braze already present, iterable,
  customer.io), ad platforms (google ads, meta ads, tiktok ads, linkedin ads),
  measurement (google tag manager, looker studio, segment, amplitude, mixpanel),
  social tooling (hootsuite, sprout social, buffer, later), CMS (contentful,
  webflow already present), and the disciplines themselves (technical-seo,
  link-building, paid-social, demand-generation, lifecycle-marketing,
  marketing-automation, generative-engine-optimization, content-marketing,
  email-marketing, influencer-marketing, copywriting, ppc).
- **Missing title aliases in `internal/classify`,** EN and RU, all resolving to the
  existing `marketing` category — no new `CategoryValues` member. `GTM Engineer`
  resolves to `sales`, joining `revops`/`sales operations`.
- **Homonym discipline.** `GTM` overwhelmingly means Go-To-Market in a posting
  ("GTM strategy", "GTM motion", "GTM Engineer"); Google Tag Manager is normally
  spelled out or named as a container, so the abbreviation resolves to go-to-market
  and the tag manager needs its full name. `GEO` means Generative Engine
  Optimization in a marketing title and geography everywhere else, so only
  disambiguated phrases resolve it.
- No new category, no schema migration, no change to any matcher mechanism.

## Capabilities

### New Capabilities

- `marketing-role-taxonomy`: the catalogue's coverage contract for marketing
  disciplines — which clusters the role dictionary must separate, which tooling the
  skill dictionary must resolve, and how the `GTM`/`GEO` homonyms are partitioned
  across the classify/roletag/skilltag layers so neither meaning leaks into the other.

### Modified Capabilities

- `skill-tag-matching`: the matcher gains one rule. Every phrase match is currently
  a *strong* match and therefore corroborates the gated single-word canonicals
  (`ai`, `automation`, `seo`). A marketing discipline named as a phrase is a
  concept, not a concrete technology, so it must tag without corroborating —
  otherwise the "AI-powered" prose that saturates marketing postings tags the whole
  population with `ai`. Named products keep corroborating.

`role-facet` and `tech-classification` are unchanged: they specify emission order,
the role catalog and the tri-state derivation, and this change only adds dictionary
content under those rules.

## Impact

- `internal/roletag/roletag.go` — named-role table and role catalog labels.
- `internal/skilltag/dictionaries.go` — canonical set, phrase aliases, and the
  ambiguity gate (`gtm` is case-scoped; `geo` never resolves as a skill).
- `internal/classify/dictionaries.go` — `categoryTable` marketing block (EN + RU)
  and the `gtm engineer` → `sales` entry placed before the bare `sales` alias.
- `cmd/gen-contracts` output — every new role slug needs a catalog label, which the
  generated web contracts carry to the role picker.
- **Reaching existing jobs:** skill and category changes need `cmd/backfill-derive`
  followed by `make reindex`; roles are index-time only and a reindex alone suffices.
  Run as a deliberate prod operation, never stacked with `reindex-companies`.
