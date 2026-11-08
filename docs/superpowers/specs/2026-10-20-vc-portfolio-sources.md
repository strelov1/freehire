# VC portfolio sources for backer collections (research, 2026-10-20)

All findings below were checked with live curl requests on 2026-10-20. Scratch files are in `/tmp/vcr/`.

## 1. How the existing backer collections get their members

- **Registry:** `internal/job/collections/collections.go` (`All`, around lines 167-286). Each `Collection{Kind: KindBacker}` has exactly one `Dataset`: a pinned `URL`+`Parse`, embedded `Data`, a `ResolveURL`, or a `Records` func that fetches and parses itself. `Dataset.Valid` enforces "exactly one".
- **yc:** `URL` is `https://yc-oss.github.io/api/companies/all.json`, parsed by `ParseYC` (open mirror of the YC directory).
- **techstars:** `URL` is `https://raw.githubusercontent.com/ark-storzhv/techstars-parser/main/TechStars.csv`, `;`-delimited, `name` column, parsed by `ParseTechstarsCSV`. It is a third-party scrape: about 3,374 rows, and the repo's last commit is **2022-11-03**, so this snapshot is stale and will never refresh. The official techstars.com/portfolio is a Next.js page backed by a Typesense search (not investigated further).
- **a16z-portfolio / a16z-speedrun:** `Dataset.Records: speedrunMembers(tier)` in `internal/job/collections/speedrun.go`. It walks `https://speedrun-talent-network.com/api/v1/companies?source=freehire&page=N` (100 per page, about 8 pages, `User-Agent: freehire/0.1 (+https://freehire.me)` is required) and keeps `tier=="a16z"` (**280** companies today) or `tier=="speedrun"` (**45**). `tier=="market"` (455) is excluded. **Finding:** this is only the subset of a16z that is on the talent network. The official a16z.com/portfolio embeds **876** companies (see section 3).
- **Matching:** `Collection.Members` (collections.go, around lines 470-520) maps each `Record.Name` through `normalize.CompanySlug` and looks it up in a map keyed by catalogue `company_slug`. Matching is **by name only. No domain is used today**, even when the source has one. A backer has no `Gate`.
- **Writer:** `cmd/import-collections/main.go` resolves every dataset, runs `Members`, then `Reconcile`s `companies.collections` and denormalizes onto `jobs.collections`. It has three abort guards: fetch/resolve error, zero records, and a tag about to lose most of its holders (`-force` overrides). `-dry-run` reports counts. Each collection's URL can be overridden with `<SLUG>_DATASET_URL`. Fetches go through `SOURCES_PROXY_URL` when it is set. `cmd/harvest-ats` re-reads the same datasets to discover boards.
- **Refresh cadence:** none. `import-collections` has **no timer**; it is run by hand after a release (freehire-ops `README.md`, "import-collections has no timer"). After it runs, both `reindex` and `reindex-companies` are needed.

## 2. Per-firm sources

| Firm | Official source | Format / endpoint | Count | Active vs exited | Domains | Difficulty / ToS |
|---|---|---|---|---|---|---|
| **Sequoia** | sequoiacap.com/our-companies | Framer site. Only about 20 "spotlight" companies are in the HTML. The full set is in Framer's **search index JSON** `https://framerusercontent.com/sites/1b1QJ6Gr5B06KwaurpRpvu/searchIndex-<hash>.json` (gzipped, about 3.8 MB). Each `/companies/<slug>` entry has `title` "X \| Sequoia Capital" and `p[]` containing the domain, "Founded/Partnered YYYY" and "Acquired/IPO YYYY". The hash changes on every publish, so it must be discovered from the page HTML (the `ResolveURL` pattern). Cross-check: `sitemap.xml` lists 431 `/companies/*` URLs. | **431** | yes: 41 Acquired, 68 IPO; the rest have no marker | **yes, 431/431** (e.g. `n.xyz`, `evy.eu`) | Easy. Server is Framer, no bot wall, robots allows all. The search index is an internal Framer asset, not a documented API. The 431 looks like a curated subset of Sequoia's total investments. |
| **Benchmark** | benchmark.com | One static page with two addresses and a Twitter link. **No portfolio is published.** | — | — | — | No authoritative source exists. Wikidata P1951 has 2 items. A hand list is the only option. |
| **Founders Fund** | foundersfund.com/portfolio | WordPress. **REST JSON** at `https://foundersfund.com/wp-json/wp/v2/company?per_page=100` (`X-WP-Total: 64`) returns `title.rendered`, slug and description. The page HTML lists the same names. | **64** (notable companies only) | no status | **no** (neither the list nor the detail pages carry the company URL) | Easy. nginx, robots `Crawl-delay: 10`. A small curated list with name-only matching. |
| **BOND** | bondcap.com/investments | Gridsome (Vue) SPA. **Static JSON**: `https://www.bondcap.com/assets/data/investments/index.json` gives `data.investments_bond.edges[].node{name,url}` (**85**) and `data.investments_kp.edges` (42, the legacy *Kleiner Perkins Digital Growth* fund, which should be **excluded** or flagged). | 85 (+42 KP DGF) | no field (exits only appear in the name text, e.g. "Brex (Capital One acquired)") | **yes** (`//akasa.com/`) | Easy. Netlify, robots only disallows `/jobs`. |
| **Index Ventures** | indexventures.com/companies/ | **Static server-rendered HTML** (Django/gunicorn). 330 `<li class="companies__relationships__list__item js-company" data-backed data-regions data-sectors>` items, each with name, `/companies/<slug>/` and an optional `<span class="ticker-symbol">` (23 have one). | **330** | only public tickers. Acquired companies seem to be dropped (Wiz is absent from the list). | on the **detail page only** (`/companies/1stdibs/` has `href="http://1stdibs.com"`), so 330 extra requests | Easy to medium. No bot wall, robots open. |
| **Lightspeed** | lsvp.com/companies | **Static HTML** (WordPress; the custom post type is not exposed in REST). `<li data-company-id data-investor="lsvp\|lsip\|both">` with `<h5>` name and an info list (Founded, Stage, Backed Since, **Status**). | **684** items (578 lsvp, 65 lsip, 31 both) | **yes**: Private 502, Acquired 95, IPO 24, Public 6, SPAC 3 | on the **detail page only** (`/company/arrcus/` has `href="https://arrcus.com"`) | Medium. Behind Cloudflare, but the curl UA got 200; robots only disallows `/wp-admin`. |
| **USV** | usv.com/portfolio | Next.js on **Sanity CMS**. The public GROQ API works without a token: `https://2r1amp8c.apicdn.sanity.io/v2021-10-21/data/query/production?query=*[_type=="company"]{name,website,url,currentStage,siteProfile}` | **262** | **yes**: `currentStage` early/growth/public/acquired/inactive | `website` on 182, `url`/`siteProfile.website` cover more | Easy. The site is behind Cloudflare, but the Sanity CDN is not. It is an undocumented-but-public CMS read API; the HTML page is the fallback. |
| **Accel** | accel.com/companies | Next.js app router on **Sanity**. Public GROQ: `https://458oembh.apicdn.sanity.io/v2021-10-21/data/query/production?query=*[_type=="company"]{name,websiteUrl,currentStatus,archived,firstInvestDate}`. The page says "(782)". | **800** docs (782 shown) | `currentStatus` true 573 / false 227. I *infer* this means current vs. past/exited; not verified. `archived` flags 16. | **`websiteUrl` on about 770** | Easy. Vercel; robots disallows `/api/` on accel.com (Sanity is a different host). |
| **Greylock** | greylock.com/portfolio | Next.js on **Sanity**. Public GROQ: `https://uz672vkd.apicdn.sanity.io/v2021-10-21/data/query/production?query=*[_type=="portfolioCompany"]{name,website,status}` | **162** | **yes**: Active 101, Acquired 39, Public 22 | **yes, 159/162** | Easy. Vercel. robots blocks only AI crawlers (GPTBot, CCBot, ClaudeBot). |
| **SoftBank Vision Fund** | visionfund.com/portfolio | Next.js pages router. **`__NEXT_DATA__`** → `props.pageProps.entry.modules[2].portfolio_companies.data[]` has `title`, `slug`, `fund` (SVF1 68 / SVF2 272), `company_status`, `country`, `sector`. | **340** | **yes**: Private 275, Public 57, Acquired 8 | on the **detail page only** (`/portfolio/guazi` has `https://www.guazi.com/`) | Easy to medium. No bot wall. (group.softbank's `/vision-fund/portfolio` returns 404.) |

### Bonus finding: a16z itself

`https://a16z.com/portfolio/` embeds about **876** company JSON objects in `data-company='{…}'` attributes. Each has `name`, `status` (Active 685 / Exits 188), **`external_url`** (876/876), stages and verticals. robots allows everything. That is three times the 280 the speedrun `tier=a16z` gives, and it has domains.

### Fallbacks checked

- **Wikidata P1951 "investor"** exists (wikibase-item), but coverage is negligible. Counts of items with P1951 = firm: Sequoia 28, Founders Fund 19, a16z 17, Accel 10, SoftBank Group 8, Lightspeed 7, Index 5, USV 5, Greylock 4, Benchmark 2, SVF 2, BOND 0. It is not usable as a membership source.
- **Crunchbase** has full coverage but is paid and its ToS forbids redistribution. Noted only; not recommended.

## 3. Recommendation

**Do first (structured JSON, domains, status; one request each):**

1. **Greylock**, **USV** and **Accel**: one Sanity GROQ GET each. They share one parser shape, so a small `sanityQuery(project, groq)` helper is a reasonable shared seam *if* all three land.
2. **BOND**: one static JSON file. Take only `investments_bond`.
3. **Sequoia**: one gzipped JSON, URL resolved from the page HTML (`ResolveURL`). Domain and exit status are present.
4. **a16z (upgrade)**: switching `a16z-portfolio` to a16z.com/portfolio would roughly triple coverage and add domains. Keep `a16z-speedrun` on the talent-network API.

**Medium (list is static, domain needs N detail fetches):** **Lightspeed** (684, has status), **Index** (330), **SoftBank VF** (340, has status). Without the detail pages they work fine name-only, which is all the matcher uses today. Keep the `Records` contract: any failed page fails the whole read.

**Weak or none:** **Founders Fund** (64 notable names, no domains; easy but small). **Benchmark** publishes no portfolio, so the only honest option is a hand-written embedded list (`Dataset.Data`, like `eastern_roots.txt`), or skip it.

**Minimal way to plug in:** add one `collections.All` entry per firm with `Kind: KindBacker` and a `Dataset`:

- `URL` + `Parse` for BOND, and for Sanity using the GROQ URL as a pinned `URL`.
- `ResolveURL` + `Parse` for Sequoia.
- `Records` for the HTML-scraped and multi-request ones.

Each `Parse`/`Records` emits `Record{Name, Meta{status, website}}`. Nothing else changes: `cmd/import-collections` picks new entries up automatically. Follow it with `-dry-run`, the real run, then `reindex` + `reindex-companies`. The web needs a brand mark per new backer slug.

Two decisions to make while doing this:

- **Exited companies:** decide whether to drop acquired/inactive ones with a `Gate`, or in the parser, using `Meta["status"]`. Acquired companies usually now hire under the acquirer's slug, so name matching would just miss them anyway.
- **Domain matching:** today matching is name-only. Every good source here carries a website. Matching on domain (e.g. against `companies.company_info` website, which `publish-logo-domains` already reads) would cut false positives for short names like "Found", "Enter", "Until", "River" (Founders Fund) or "Because" and "Bird" (Index). Note it as the seam; it is not required for a first cut.

Since none of these are on a timer, they refresh only when someone reruns `import-collections`. If backer lists should stay current, a monthly timer for `import-collections` is the natural follow-up. The existing large-loss abort guard already protects against a broken upstream.
