## Why

vouch.careers is a hosted careers platform (referral-driven hiring) that puts each
company's jobs at `vouch.careers/companies/<companyId>`. It is a Next.js App Router
app that server-renders every posting into the RSC flight stream, keyless. We do not
ingest it today, so live vacancies on these boards (e.g. Laine's engineering roles)
are missing from the catalogue. A spike VALIDATED that the company page inlines the
full, structured job objects in the flight.

## What Changes

- Add a `vouch` source adapter (`internal/sources/vouch.go`) over the keyless
  `https://vouch.careers/companies/<board>` company page. The board id is the URL
  company cuid; the adapter is board-based (not boardless) and stays in the source
  facet.
- The adapter fetches the **single company page**, decodes the Next.js RSC flight,
  and extracts the inlined `"listings":[…]` JSON array — no per-job detail fan-out,
  because each listing object already carries its full body (`title`, `pitch`,
  `must`/`nice` HTML, `employmentType`, `company.name`, `locations`, `publishedAt`,
  and the `activated`/`draft`/`unlisted` state). Only **live** postings (activated,
  not draft, not unlisted) are emitted.
- Work mode is a STRUCTURED signal: `employmentType` carries "Remote"/"Hybrid"/
  "On-site" tokens, mapped to the work-mode vocabulary.
- Reuse the existing Next.js flight primitives from `deel.go` (`bracketSlice` and
  the flight decoder), extracting the generic flight decoder into a shared,
  neutrally-named helper now that a second adapter consumes it.
- Register the adapter in `sources.All` (one line). Add `sources/vouch.yml` seeded
  with the validated Laine board.

## Capabilities

### New Capabilities
- `vouch-source`: crawling one vouch.careers company page into the catalogue by
  decoding the Next.js RSC flight and mapping each inlined live listing to a `Job`.

### Modified Capabilities
<!-- none: no existing spec's requirements change. The deel flight-decoder rename is an
     internal refactor with no behavior change (deel's spec/behavior is unchanged). -->

## Impact

- New file `internal/sources/vouch.go` (+ test) and one line in
  `internal/sources/source.go` (`All`).
- A small internal refactor in `internal/sources/deel.go`: rename the generic flight
  decoder to a shared name (`decodeNextFlight`/`nextFlightPush`), no behavior change,
  covered by the existing deel tests.
- New board file `sources/vouch.yml`; a cron schedule is an ops follow-up.
- No schema, API, or migration changes; read-only over public HTTP.
