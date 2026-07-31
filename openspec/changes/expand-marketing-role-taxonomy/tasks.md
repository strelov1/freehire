## 1. Classify — title aliases (EN + RU)

- [x] 1.1 Add the `gtm engineer` / `go-to-market engineer` family to `categoryTable`
      resolving to `sales`, placed before the bare `sales` alias; test that
      "GTM Engineer" classifies as `sales` and "Sales Manager" is unaffected
- [x] 1.2 Extend the marketing block with the missing EN phrase aliases (growth
      marketing, demand generation, performance marketing, lifecycle marketing,
      paid media, paid social, email marketing, influencer marketing, community
      manager, content marketing, marketing operations, product marketing), all
      resolving to `marketing`
- [x] 1.3 Add the RU marketing aliases (таргетолог, контент-менеджер,
      бренд-менеджер, пиар, интернет-маркетолог, перформанс-маркетолог,
      email-маркетолог) as full surface forms
- [x] 1.4 Regression-pin the technical titles the new aliases must not claim:
      "Growth Engineer", "Content Platform Engineer", "Geo Data Analyst" keep
      their pre-change category and `is_tech`
- [x] 1.5 Assert `vocab.CategoryValues` and `vocab.NonTechCategories` are unchanged
      (already covered by `TestCanonicalValuesAreInVocabulary` + the vocab partition
      test — no new test written)

## 2. Roletag — granular marketing roles

- [x] 2.1 SEO cluster: `technical_seo_specialist`, `content_seo_specialist`,
      `link_building_specialist`, `seo_analyst`, with labels; longest-alias-first
      ordering keeps them off the existing `seo_specialist`
- [x] 2.2 GEO cluster: one `geo_specialist` slug collapsing the spelled-out,
      AEO and GSO forms plus the bound `geo specialist` / `geo manager`; assert
      "Geo Data Analyst" and "Geospatial Engineer" emit no GEO role
- [x] 2.3 SMM cluster: `smm_manager`, `community_manager`, `paid_social_specialist`,
      `content_creator`
- [x] 2.4 Commercial cluster: `gtm_engineer`, `demand_generation_manager`,
      `lifecycle_marketing_manager`, `performance_marketer`,
      `marketing_operations_manager`, `brand_manager`,
      `pr_manager`, `influencer_marketing_manager`, `copywriter`,
      `marketing_analyst`
- [x] 2.5 Assert every new slug has a catalog label and that the existing
      `growth_engineer` role still resolves for "Growth Engineer" (catalog side
      already covered by the package's invariant tests; the guard lives in 2.4)

## 3. Skilltag — marketing tooling and disciplines

- [x] 3.1 Search tooling as strong (ungated) aliases: semrush, ahrefs, screaming
      frog, google search console, moz; assert they corroborate the gated `seo`
      canonical on a posting carrying no engineering technology
- [x] 3.2 Lifecycle/email and ad platforms: klaviyo, mailchimp, iterable,
      customer.io, google ads, meta ads, tiktok ads, linkedin ads; assert
      "Google Ads" does not additionally emit an unrelated Google canonical
- [x] 3.3 Measurement and social tooling: looker studio, amplitude (gated),
      hootsuite, sprout social, contentful. Segment/Buffer/Later deliberately
      excluded — ordinary English words in marketing prose
- [x] 3.4 `GTM` resolves to the **go-to-market** canonical, not the tag manager:
      the abbreviation is go-to-market in a posting, while Google Tag Manager needs
      its spelled-out name or the `gtm container` form. No `GTM` acronym entry
- [x] 3.5 Discipline canonicals as phrase aliases: technical-seo, link-building,
      paid-social, demand-generation, lifecycle-marketing, marketing-automation,
      generative-engine-optimization, content-marketing, email-marketing,
      influencer-marketing, copywriting, ppc (phrase forms only — `sem` is
      deliberately excluded as a PT/ES preposition collision)
- [x] 3.6 Assert separator-insensitivity holds for the new multi-word canonicals
      ("paid-social" / "paid social" / "paid_social")

- [x] 3.7 Matcher rule: `nonCorroboratingPhrases` + the `standalone` bucket in
      `Parse`, so a discipline phrase tags on its own but never rescues a gated
      single-word canonical (`ai`, `automation`). Named products keep corroborating

## 4. Contracts and integration

- [x] 4.1 Run `cmd/gen-contracts` and verify every new role slug reaches the web
      contracts with its label
- [x] 4.2 `go build ./... && go vet ./... && go test ./...` green

## 5. Reaching existing jobs (prod, run deliberately)

- [ ] 5.1 Run `cmd/backfill-derive` on prod to re-derive `skills` and `category`
- [ ] 5.2 Run `make reindex` to populate `roles` and pick up re-derived skills —
      never stacked with `reindex-companies`
