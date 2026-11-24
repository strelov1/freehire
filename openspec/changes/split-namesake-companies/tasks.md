## 1. Board domain evidence

- [ ] 1.1 Migration: `boards.company_domain` (nullable bare host) and `company_board_slugs`
      (provider, board → slug, reason, decided_by, decided_at), no FK to `companies`
- [ ] 1.2 Fill `company_domain` from ATS payloads that state the employer site (Ashby
      organization website, Recruitee/Teamtailor company site, Greenhouse board meta);
      refuse ATS hosts via `atsboard`
- [ ] 1.3 `cmd/report-company-namesakes`: hiring slugs whose boards name 2+ domains, or
      whose `company_info.website` matches none of its board domains; entry in workers.md

## 2. Domain precedence

- [ ] 2.1 Company-info writers refuse a website that contradicts the slug's board domains
- [ ] 2.2 `ListCompaniesBySiteHost` / `by-domain` prefer the slug whose board names the host
- [ ] 2.3 Integration tests: exa.ai resolves to the Exa Labs slug, gomotive.com to GoMotive

## 3. The `different` decision

- [ ] 3.1 `companyrelation`: a `different` proposal names a board; approval elects the
      frozen slug, writes the override, moves the board's open postings, reconciles both rows
- [ ] 3.2 `pipeline.Runner` resolves the board override before the name, in the one
      resolved map the coverage gate and the upsert share
- [ ] 3.3 Moderation tab: show board, domain and sample postings for a `different` proposal
- [ ] 3.4 Correct `motive`, `exa`, `parallel`; update docs/agents/company-identity.md
