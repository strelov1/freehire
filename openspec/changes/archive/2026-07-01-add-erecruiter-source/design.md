## Context

Spike (memory `hire-erecruiter-adapter-spike`) validated that eRecruiter Polska exposes a
fully keyless public per-company job board on `skk.erecruiter.pl`:

- **Board id = `cfg`** (32-hex), embedded in a company careers page as
  `<script src="https://skk.erecruiter.pl/Code.ashx?cfg=<hex>">`.
- **List**: `GET GetHtml.ashx?cfg=<cfg>&grid=rows&pn=<page>` returns a JSONP body
  `({"htm":"<tr ...><td>..</td></tr>"})`. Each `<tr>` carries `jobOfferId`, `offerId`,
  `externalJobOfferId`, `externalJobOfferRegionId`, `comId`, and three `<td>` cells
  (position, category, city). Page size is 20. A page past the end returns a hidden
  marker row `<tr tp='2' tr='<TOTAL>' pn='<n>' ps='20'>`, so `tr` = total result count.
- **Detail**: `GET Offer.aspx?oid=<offerId>&cfg=<cfg>&ejoId=<externalJobOfferId>&ejorId=<externalJobOfferRegionId>&comId=<comId>`
  returns full HTML (title, company, location, description) and an "Aplikuj" button
  linking to `system.erecruiter.pl/FormTemplates/RecruitmentForm.aspx?WebID=<hex>` — the
  same form justjoin links to.

The recruiter-side surfaces (`system.erecruiter.pl/Career`, `/Offers`, `vacancyapi`) are
OIDC/401-gated and are NOT used. The adapter slots into the existing `sources` registry
and `pipeline.Runner` write path unchanged.

The discovery gap: justjoin only exposes per-job `WebID` apply forms, which carry no
`cfg`. Onboarding the 161 justjoin companies therefore needs a separate step that resolves
each company's `cfg` from its own careers page.

## Goals / Non-Goals

**Goals:**
- A keyless, board-based `erecruiter` adapter (list + per-offer detail) that ingests one
  company board per configured entry, deduping on the stable `externalJobOfferId`.
- A `cmd/harvest-erecruiter` tool that turns a list of company careers URLs (or domains)
  into validated `sources/erecruiter.yml` entries.

**Non-Goals:**
- Automatically mapping every justjoin `WebID` to a `cfg` (no such mapping exists) —
  discovery is domain/careers-page driven, run by the harvester, not the adapter.
- Ingesting via the recruiter-side authenticated eRecruiter API.
- Self-close signals from eRecruiter (handled by the standard unseen-jobs sweep like any
  other board source).

## Decisions

- **Board id is the `cfg` token.** It is stable per company and is the only identifier the
  public endpoints accept. `sources/erecruiter.yml` entries are `company` + `board`=cfg.
  Alternative — deriving from `comId` — rejected: `comId` is not accepted by the public
  list/detail endpoints, only `cfg` is.
- **Two-step list→detail fetch.** The list row lacks a description, so the adapter fetches
  each offer's `Offer.aspx` detail for the body. Alternative — indexing title-only from
  the list — rejected: it would persist bodyless postings, degrading search/enrichment.
  This mirrors existing HTML detail adapters (Avature, SuccessFactors `GetHTML` seam).
- **JSONP unwrap via `GetText`.** The list endpoint returns `({"htm":"…"})`, neither clean
  JSON nor a clean HTML document. The adapter reads it with `GetText`, strips the wrapping
  parens/semicolon, JSON-decodes the `htm` string, then parses the inner `<tr>` rows.
- **Pagination stops on the reported total.** The first page's marker row exposes `tr`
  (total); the adapter pages `pn=1..ceil(tr/20)` and also stops early if a page yields no
  offer rows, under a bounded page cap (backstop against a pathological feed) — same shape
  as the Traffit adapter's paginated collection.
- **`ExternalID` = `externalJobOfferId`.** Stable across recrawls; the canonical detail
  URL is the `Offer.aspx` URL. The `WebID` apply form is captured as the apply URL when
  present but is not the identity (forms 404 once a job closes).
- **Harvester resolves `cfg` from careers pages.** Given a company careers URL, fetch it
  with `GetText`, regex-extract `Code.ashx?cfg=<hex>`, then probe
  `GetHtml.ashx?cfg=<hex>&grid=rows&pn=1` and keep the token only if it returns offer rows.
  Same live-validate-then-emit shape as `cmd/harvest-boards` / the Traffit prober.

## Risks / Trade-offs

- **cfg discovery coverage** → The harvester needs a company's real careers URL; blind
  guessing fails (Fabrity/Medicover careers pages 404/403 on guessed paths). Mitigation:
  seed the harvester from the justjoin company list (names → domains) and accept partial
  coverage; missing companies still arrive via justjoin as today.
- **HTML detail parsing is brittle** → eRecruiter's `Offer.aspx` markup can change.
  Mitigation: parse defensively (skip a posting whose title/description can't be extracted
  rather than failing the board), and cover the parse with a fixture test.
- **Non-IT noise** → Boards carry all company jobs (e.g. cleaning roles). Mitigation: none
  needed — the standard classify/skilltag dictionaries derive facets and IT filtering
  happens downstream as for every source.
- **Per-offer detail fetches add load** → one request per posting. Mitigation: bounded by
  each board's real size (tens, not thousands); acceptable at cron cadence.

## Migration Plan

- Ship adapter + registry line + empty/seed `sources/erecruiter.yml`; no schema changes.
- Wire a per-provider ingest cron for `sources/erecruiter.yml` (one schedule, like other
  board files). Rollback = drop the cron entry and the registry line; no data migration.
- Run `cmd/harvest-erecruiter` over the justjoin company set to populate the board file
  incrementally.

## Open Questions

- Does `Offer.aspx` expose a reliable posted-at date? If not, postings fall back to ingest
  time via the existing `effectivePostedAt` handling (acceptable).
- Best seed for the harvester: reuse the justjoin-mined company list, or fold eRecruiter
  cfg detection into the existing `domain-ats-harvest`? Deferred to harvest execution.
