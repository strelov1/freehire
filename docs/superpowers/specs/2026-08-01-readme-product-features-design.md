# README: document the product half

**Date:** 2026-08-01
**Status:** approved, ready for planning

## Problem

`README.md` describes freehire as a catalogue and an ingest pipeline: Why → Stack
→ Quick start → Workers → Layout → Sources → Adding a source. Every section
below "Why freehire?" is about getting postings *in*.

Roughly a third of `internal/` serves what happens *after* a posting is found —
`cv`, `cvedit`, `cvmatch`, `matchanalysis`, `experience`, `tracerlink`,
`jobtracking`, `appevent`, `inbox`, `mailingest`, `maillink`, `assistant`,
`referral`, `ghost`, `subscription`, `savedsearch`. None of it appears in the
README. A reader learns the project aggregates jobs and nothing about the
workspace built on top: the application tracker, the mail inbox, CV tailoring,
tracer-link analytics, the in-app agent, or interview rehearsal.

This costs twice: a visitor underestimates the project, and a contributor sees
only one seam to contribute to (add a source).

## Decision

Two artefacts, deliberately split by depth.

1. **A short block in `README.md`** — a five-row table keyed by the job-seeker
   funnel, each row linking into the new document. README stays ~405 lines.
2. **`docs/features.md`** — a hybrid reference, ~150 lines: what each feature
   does for a person, plus where it lives in the tree.

Rejected: rewriting the README's positioning (more churn than the gap warrants),
and a product-only page (would duplicate the `/features/*` landings on the site).

## `README.md` changes

Four edits, all additive:

1. **New section `## Beyond the catalogue`**, placed directly after
   "Why freehire?" and before "Stack". A table of five rows — Find, Apply,
   Track, Ask, Build on — one line of features each, with a lead-in sentence and
   a link to `docs/features.md`.
2. **Nav line (line 14)** — add `[Features](docs/features.md)` between `Sources`
   and `API`.
3. **`## Layout`** — the tree lists only pipeline packages. Add the load-bearing
   product ones: `cv/`, `cvedit/`, `assistant/`, `inbox/`, `userjob/`,
   `experience/`.
4. **"Why freehire?" last bullet** — it already says "per-user application
   tracking"; widen it to name the workspace and point at the new section.

Untouched: the header, the tagline, the counts, and every source table.

## `docs/features.md` structure

Five sections following the funnel. Per feature: a heading, 2–3 sentences of
what it does and why it is built that way, then one pointer line.

```
### CV tailoring

Rewrites one CV against one vacancy, requirement by requirement, and invents
nothing: every edit must be backed by an atom in the experience bank, and the
check lives in the service path rather than in a prompt. Autopilot walks the
whole vacancy in a single pass and snapshots the CV first, so the run is
undoable.

Live: /tailor/<job> · Code: `internal/cvedit`, `internal/assistant` · Deep dive: [internal/cvedit/AGENTS.md](../internal/cvedit/AGENTS.md)
```

**Find** — faceted search over the catalogue; collections and role landings;
saved searches with subscription digests (email + Telegram); market analytics
(`/insights`, `/trends`); the ghost-job signal.

**Apply** — the CV builder (Typst templates, PDF, previews, headshot); the
deterministic CV↔vacancy score and the Job Match tab; the LLM fit analysis;
CV tailoring plus autopilot; the experience bank and its provenance rule;
tracer links; PII masking; referrals.

**Track** — the application board and its stages; the hosted mail inbox with
automatic linking of replies to applications; the application event ledger;
reminders and notification channels.

**Ask** — the in-process assistant: five presets (`chat`, `browse`, `profile`,
`tailor`, `interview`), the tool contract, dictation, and the fact that tools
act as the authenticated caller rather than under a minted credential.

**Build on it** — the keyless public API, the CLI, the browser extension,
ChatGPT Actions, and crowdsourced board contributions.

## Accuracy rules

These bind the implementation:

- **Every link is verified before commit.** Each `internal/*/AGENTS.md` target
  must exist on disk; each `Live:` path must exist under `web/src/routes`.
  A README that links into nothing is worse than a README that omits the
  feature.
- **LLM-gated features are marked as such.** Tailoring, fit analysis, Telegram
  extraction and the assistant need `LLM_*` configured and stay disabled in a
  self-host without it — the README already sets this precedent for OAuth.
- **The ghost-job signal is described as a signal**, not a verdict.
- **No feature is listed that is not reachable in production.** Anything found
  to be behind a flag or unshipped is dropped rather than hedged.

## Out of scope

Screenshots and GIFs per feature; changes to the `/features/*` landing pages on
the site; any code change; the source tables and their counts.

## Verification

- `docs/features.md` exists, is under ~160 lines, and every one of its links
  resolves (checked by listing the targets, not by eye).
- `README.md` renders with the new section and nav link, and stays under 420
  lines.
- No file outside `README.md` and `docs/features.md` is modified.
