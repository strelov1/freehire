## Context

A spike against NoFluffJobs established:

- `GET https://nofluffjobs.com/api/posting` returns a single keyless JSON object `{ postings: [...] }`
  — ~8 400 postings, ~60 MB. No pagination. The 60 MB body exceeds the size-capped `GetJSON`, so it
  must be read via `GetStream` (as isolvedhire does for its large sitemap).
- Each listing posting carries: `id` (stable, e.g. `gcp-…-verita-hr-Kraków`), `url` (ASCII slug,
  e.g. `gcp-…-verita-hr-krakow`), `name` (company), `title`, `posted`/`renewed` (epoch ms),
  `technology` (a skill string), `seniority` (array, e.g. `["Mid"]`), `category`, `fullyRemote`,
  `salary`, and `location.places[]` (`{city, country{name}}`). **No description.**
- `GET https://nofluffjobs.com/api/posting/<url>` (keyless, ~12 KB) carries the description across
  `requirements.description` (HTML), `details.description` (HTML), `requirements.musts` (skills),
  and `specs.dailyTasks`.

This is the justjoin shape (`internal/sources/justjoin.go`): a boardless aggregator whose list omits
the body, hydrated per-posting via a detail endpoint, using the pipeline's `HydratingSource`
(`FetchNew(ctx, entry, seen)`) so only new postings pay the detail cost.

## Goals / Non-Goals

**Goals:**
- A keyless, boardless, aggregator `nofluffjobs` adapter mirroring justjoin's hydrating pattern.
- Structured facets (skills, seniority) taken from the **listing** — no detail request needed for them.
- Reuse `GetStream`, `skilltag.Parse`, `enrich.SeniorityValues`, `sanitizeHTML`; no new shared helper.

**Non-Goals:**
- A freshness/keyword filter. The user chose full detail hydration; `FetchNew`'s seen-set already
  bounds steady-state cost to new postings, and the standard sweep soft-closes anything that ages out.
- Structured salary mapping. justjoin does not map salary either; the description + enrichment cover it.
- Category mapping. Like justjoin, nofluffjobs `category` is a stack/language tag that doesn't pin a
  freehire role category — the title dictionary decides it.

## Decisions

- **Stream the listing.** `GetStream(ctx, listURL, "application/json", func(r) { json.NewDecoder(r).Decode(&resp) })`
  decodes the 60 MB `{postings:[…]}` past the `GetJSON` size cap. *Alternative rejected:* `GetJSON` —
  it caps and truncates the body.

- **Facets from the list, description from the (new-only) detail.** `toJob` maps every list field
  including skills (`skilltag.Parse(technology)`) and seniority. `FetchNew` fetches `/api/posting/<url>`
  only for postings the seen-predicate reports as new, assembling the description from
  `details.description` + `requirements.description` (both HTML) and sanitizing; a seen posting keeps
  its list-only job so the pipeline refreshes liveness without wiping the previously-hydrated body.
  *Alternative rejected:* detail-fetch every posting every run — ~8 400 requests per crawl; the
  seen-set is exactly the justjoin mitigation.

- **ExternalID = `id`; URL from `url` slug.** `id` is the stable dedup key; the ASCII `url` slug builds
  both the canonical page (`https://nofluffjobs.com/job/<url>`) and the detail request. A posting with
  no `id`, no `url`, or no company is dropped (empty id/URL/company would break dedup or the slug).

- **Seniority mapping.** NoFluffJobs names the middle grade `Mid`; map `mid`→`middle`, then keep the
  value only if it is in `enrich.SeniorityValues` (unrecognized grades drop rather than being guessed)
  — the same rule justjoin uses.

- **Boardless aggregator, keyless.** `boardless()` + `aggregator()` markers; the config entry is a
  single boardless line. One line in `sources.All`. No key. No proxy unless the prod IP is
  rate-limited (a later config flip).

## Risks / Trade-offs

- **60 MB listing per run.** → `GetStream` avoids the size cap; the decode holds ~8 400 structs in
  memory briefly (acceptable). The response is uncompressed JSON; the standard client sends
  `Accept-Encoding: gzip`, so the wire transfer is far smaller.
- **First-run detail fan-out (~8 400 requests).** → `FetchNew`'s seen-set makes this a one-time cost;
  steady state hydrates only new postings. If the site rate-limits, `proxiedProviders` is the seam.
- **Polish-language descriptions.** → Skill tags and structured seniority survive; enrichment prose
  is weaker but the structured facets carry value. Accepted (same as justjoin).

## Migration Plan

- No DB migration, no API change, no new dependency, no key.
- Deploy: add a `cmd/ingest sources/nofluffjobs.yml` cron schedule in freehire-ops.

## Open Questions

- Does the prod datacenter IP get rate-limited on ~8 400 first-run detail requests? Resolve with a
  manual prod run; `proxiedProviders` enrollment is the fallback.
