## Context

eArcu is a UK multi-tenant ATS whose clients each run on a custom careers host
(`careers.<company>.com`) and expose a keyless RSS feed at `/jobs/rss`. A live probe of
`careers.cambridge.org/jobs/rss` returns a valid RSS 2.0 document (38 `<item>`s) where each
item carries:

- `<title>` — e.g. `Transfer Pricing Manager (7367)` (trailing `(ref)` is the eArcu reference)
- `<link>` / `<guid>` — the detail URL `https://<host>/jobs/vacancy/<slug>/<id>/description/`
- `<pubDate>` — RFC1123-style timestamp (`Tue, 07 Jul 2026 15:53:15 Z`)
- `<description>` — a `Key: value, …` prefix (`Country: UK, … Salary: £…, Location: Cambridge,
  eArcu Vacancy Reference: 7367`) **followed by the full posting body** inside a
  `<div id="rssjobdesc">…</div>` (HTML-escaped in the feed)

Detail pages additionally embed a schema.org `JobPosting` JSON-LD block, so the existing
`ldJobPosting`/`sanitizeHTML` helpers give a clean fallback body. This maps almost exactly
onto the existing **Personio** adapter (XML feed with inline body + JSON-LD detail fallback);
eArcu differs only in that the board is the full host rather than a subdomain slug.

## Goals / Non-Goals

**Goals:**
- One keyless feed request per board yields all open postings with inline bodies.
- Reuse existing transport (`XMLGetter` + `HTMLGetter`) and helpers (`sanitizeHTML`,
  `ldJobPosting`, `parseRFC…`, `isRemote`) — no new HTTP-client capability.
- Stable dedup identity from the posting id in the detail URL.
- Ship a `sources/earcu.yml` seeded only with **live-validated** hosts.

**Non-Goals:**
- Bulk harvest of the full eArcu customer roster (follow-up discovery; this change seeds
  what validates live, starting from `careers.cambridge.org`).
- Structured facet extraction (WorkMode/Seniority/Category/Skills) from the feed's
  `Key: value` prefix — left empty so the pipeline's dictionaries decide, matching Personio.
- Applicant-side flows; the adapter is read-only over the public feed.

## Decisions

- **Provider key `earcu`, board = full careers host.** `Fetch` builds `https://<board>/jobs/rss`.
  The board is the host verbatim (e.g. `careers.cambridge.org`), so the adapter is
  board-based (not boardless) and multi-tenant — it lists in the source facet with no extra
  wiring. Config validation already requires a board for non-boardless providers.
- **RSS parse via `GetXML`.** Decode `<rss><channel><item>` with a struct mirroring the
  fields above. `pubDate` → `PostedAt` (parse RFC1123 with a `Z`-zone tolerance; nil on
  failure, never fatal).
- **ExternalID from the URL path.** Regex the `/vacancy/<slug>/<id>/description/` id out of
  `<link>`. Drop items with no extractable id (spec: never emit an empty key). Prefer the URL
  id over the `eArcu Vacancy Reference` in the description, because the URL id is what
  identifies the posting page.
- **Location from the description prefix.** Parse `Location: …` (and fall back to `Country: …`)
  out of the `Key: value` prefix that precedes `rssjobdesc`. Feed `Remote`/`WorkMode` stays
  heuristic-free: leave `WorkMode` empty and set `Remote` only via the shared `isRemote`
  location heuristic, so provenance stays clean (matches Personio).
- **Body: feed-first, detail JSON-LD fallback.** Extract the `<div id="rssjobdesc">` inner
  HTML from the (unescaped) description and `sanitizeHTML` it. If empty, GET the detail URL
  and use its `JobPosting` JSON-LD `description`; a failed detail fetch is non-fatal.
- **Title cleanup.** Strip a trailing ` (\d+)` reference suffix so titles read cleanly; keep
  the rest verbatim.
- **Transport interface.** Define `earcuHTTP interface { XMLGetter; HTMLGetter }` and
  `NewEarcu(c earcuHTTP) Source`, mirroring `personioHTTP`/`NewPersonio`. Tests use the
  existing `fakeHTTP` (which already implements both).

## Risks / Trade-offs

- **Single confirmed host at ship time.** Only `careers.cambridge.org` is verified so far;
  the seed file's value scales with host discovery. Accepted: the adapter is correct and
  reusable now, and the roster grows as follow-up harvest validates more hosts (same path
  every other adapter took). Each host is validated live before it enters `earcu.yml`.
- **Feed markup drift.** The inline body lives in a `rssjobdesc` div and the metadata in a
  `Key: value` prefix — both eArcu conventions that could change. The JSON-LD detail fallback
  hedges the body; location parsing degrades gracefully (empty location, never a crash).
- **Custom-domain edge protection.** eArcu hosts sit behind Cloudflare; the default Go
  fingerprint reached `careers.cambridge.org` fine, so we start on the standard shared client
  and only escalate to the Chrome-fingerprint transport (as bayt/EPAM do) if a probed host
  returns 403. Recorded as a per-host validation step, not pre-emptive complexity.
