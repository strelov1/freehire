## Context

See proposal.md for the discovery, the motivation for a dedicated change rather than folding
this into the harvesting work that found it, and the revision note explaining why Eightfold and
Gupy were dropped after review. `internal/ingest/atsboard/board.go`'s `atsBoards` table already
lists ~15 `subdomain`-mode platforms following the exact shape Keka needs (`<tenant>.<apex>` →
board `<tenant>`, canonical collapses to the bare tenant host).

## Goals / Non-Goals

**Goals:**
- Recognize Keka's standard multi-tenant host, matching how every other `subdomain`-mode entry
  already behaves.

**Non-Goals:**
- Eightfold and Gupy (see proposal.md's revision note — their ingest adapters key on something
  other than the bare subdomain label, so `subdomain` mode would derive a board id neither
  adapter can ever crawl).
- Paycom (see proposal.md — deliberately excluded, a separate decision).
- Any vanity/custom-domain instance of Keka — none observed in the source inventory or in the
  adapter's own board-catalog rows; every tenant is on the vendor's own domain.

## Decisions

**Mode: `subdomain`, no new extraction mode.** Keka serves every tenant at `<tenant>.keka.com`
with no path segment carrying additional identity — the same shape `recruitee.com`,
`bamboohr.com`, and a dozen others already use. Adding a new mode for a shape an existing one
already covers would just be a second implementation of the same rule.

**Source key**: `keka` — verified against the adapter's own `Provider()`
(`internal/ingest/sources/keka.go`), per `atsboard`'s own load-bearing rule that the source
MUST be the provider key the catalogue uses.

**A regression test guards the one non-tenant host found**: `app.keka.com` is Keka's own
product host, already declined by the pre-existing generic `"app"` entry in `platformLabels`
(shared across every subdomain-mode platform) — `TestRecognize/keka_platform_app_host_not_a_tenant`
pins that this stays true rather than relying on it silently.

## Risks / Trade-offs

- **Widens what the paid contribution flow rewards.** This is the change's whole point, not a
  side effect — see proposal.md. The risk it displaces (a silent widening bundled into
  unrelated work) is exactly what `atsdetect`'s own guard test exists to catch, and this change
  is the deliberate response that guard asks for.
- **A board-id-shape mismatch is a real, demonstrated failure mode, not a hypothetical one.**
  The Eightfold/Gupy reversion above is the evidence: "same URL shape as other subdomain
  platforms" is necessary but not sufficient — the derived board id must also be what the
  target adapter's `Fetch` actually expects. Any future addition to `atsBoards` needs that
  check made explicitly, not inferred from the shape resembling existing entries.
