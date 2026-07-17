## Context

The full brainstormed design lives at `docs/superpowers/specs/2026-07-17-cv-tailoring-design.md`
(local-only per repo convention). This document captures the OpenSpec-level decisions for the
**freehire-repo portion** only.

Current state the change builds on:
- `cvs` table (`internal/cv/`, migration `0024_cvs.sql`) with `data jsonb` (`cv.Document`) and a
  **nullable `job_id`** the migration comment marks as "a seam for the follow-up per-vacancy
  tailoring phase". `cv.Document.Sanitize()` is already documented as "in the follow-up tailoring
  phase — fed to an LLM, so Sanitize is both the persistence guard and the prompt-injection guard".
- `cv.Seed(structured)` builds a `Document` from the structured résumé (pure mapping).
- `jobfit.Analysis` cached per (user, job) in `user_job_analysis`; requirements carry
  `covered / synonym-only / missing-have / missing-gap` statuses.
- `api_keys` (SHA-256 hashed) with `RequireAuthOrKey` Bearer auth.
- CV routes are beta-gated via `cvGate`; the agent is `beta_tester`-gated upstream.

Constraint: this change's edit scope is the `hire` repo only. The `freehire cv` CLI subcommands
(`freehire-cli` repo) and the `cv-tailoring` agent skill (`freehire-agent` repo) are companion
changes tracked in their own repos; this change delivers the API + model + contracts + web CTA they
depend on.

## Goals / Non-Goals

**Goals:**
- Turn `cvs.job_id` into the two-tier base/tailored model.
- A pure, sanitized, field-level patch transform (`cv.Apply`) and its `PATCH` endpoint.
- A bootstrap endpoint that seeds/creates CVs, gates on a cached analysis, and mints a scoped key.
- A tailoring-context read over the cached analysis.
- A fit-page CTA that opens the tailoring session.

**Non-Goals:**
- The `freehire cv` CLI and the roy `cv-tailoring` skill (separate repos).
- An item-level **experience bank** — noted seam; the base CV is the proto-bank.
- Any change to how `jobfit.Analysis` is computed (it is only read).
- The full split-preview assistant UI polish beyond wiring the CTA and preview render.

## Decisions

**D1 — Live agent (roy) drives the dialogue; freehire owns the CV state.**
Alternative: a server-side `jobfit`-style prompt-chain inside freehire. Rejected: the user wants a
free-form interactive session, and roy already has `hire_token` SSO, shadow-user provisioning, and a
`Bash(freehire:*)`-confined tool model. freehire stays the single source of truth; the agent edits
through the CLI→API, holding no durable state and touching no filesystem (roy's `Edit`/`Write` stay
denied).

**D2 — Field-level patches, not whole-document saves.**
Alternative: `freehire cv save` replacing the entire document. Rejected: an LLM re-emitting the full
document each turn risks silently dropping untouched sections and burns tokens. Both reference
projects (ai-job-search, career-ops) chose structured field edits for auditability. `cv.Apply(doc,
patch)` is pure (mirrors `cv.Seed`); the caller sanitizes. Out-of-range addressing is a 422, never a
silent no-op.

**D3 — Two-tier CV = two `cvs` rows.** Base CV `job_id = NULL`; tailored CV `job_id = <vacancy>`,
copied from base at bootstrap. Reuses the existing table and the `ON DELETE SET NULL` already on
`job_id`. The base CV doubles as the proto experience-bank.

**D4 — Honest wall.** New facts are never fabricated. The freehire side enforces what it can observe:
the tailor-context read exposes `missing-have` vs `missing-gap` so the agent can reframe vs ask, and
base-CV edits are a distinct, explicit action (patching the `job_id = NULL` row). The
"ask-before-adding" behavior itself lives in the agent skill (separate repo) but depends on this
classification being exposed.

**D5 — Short-lived owner-scoped API key for the CLI credential.**
Alternative: pass the raw `hire_token` JWT as Bearer. Rejected: `RequireAuthOrKey` expects an API
key as Bearer, and a revocable, short-lived key is safer than handing the session cookie to a
subprocess. The freehire `api_keys` table has no per-endpoint scope column, so "scoped" here means
owner-scoped only (the key resolves to the user; the CV endpoints' own owner checks confine it) —
short-lived is real (2h TTL via the existing `expires_at`). Minted at bootstrap, returned as
`cli_token`.

**Key delivery (companion, cross-repo).** freehire only mints and returns `cli_token`; how it reaches
the CLI is the roy session bootstrap's job. Preferred: the bootstrap writes the token into the
session home in the `freehire-cli` config format (`~/.freehire`) so `freehire cv …` auto-authenticates
on first run, rather than only injecting `FREEHIRE_TOKEN` into the process env (which exposes the
secret to every child process). The exact config format lives in the `freehire-cli` repo; this change
just returns the token.

**D6 — In-repo: CTA → tailored CV editor. Live split preview is cross-repo.** The fit-page CTA
bootstraps and `goto`s the tailored copy in the existing `CvEditor` (`/my/cvs/[id]`), where the CV
is viewable/renderable (PDF via `cvPdfUrl`). A live split (chat + preview refreshed on turn
boundaries) needs the roy agent to accept **session context** on create — today `/my/assistant`'s
`createSession` sends `{}` and the agent (agent.freehire.dev, separate repo) decides everything, so it
cannot be seeded with a CV id / verdict. That interactive loop is the cross-repo companion
(`freehire-agent` context-on-create + `freehire-cli` `cv` commands); this change lands the entry
point and the tailored artifact it opens.

## Risks / Trade-offs

- **The honest wall cannot be fully enforced server-side** (freehire can't know if a fact was
  candidate-confirmed) → Mitigation: expose the `missing-have`/`missing-gap` split and keep base-CV
  edits explicit; the enforcing prompt lives in the reviewed agent skill; `Sanitize` still bounds
  everything persisted.
- **Cross-repo coordination** (API must ship before CLI/skill can use it) → Mitigation: this change
  is contract-first and independently shippable; companions land after.
- **Scoped-key leakage into a subprocess env** → Mitigation: short TTL + narrow scope + revocable via
  existing `api_keys` machinery.
- **Whole-document copy at bootstrap drifts from base** (base edited later) → acceptable: a tailored
  CV is a point-in-time projection by design; not kept in sync.

## Migration Plan

Additive only — no schema migration needed (`cvs.job_id` already exists). Deploy freehire (API +
contracts + CTA) behind the beta gate; then land the `freehire-cli` and `freehire-agent` companions.
Rollback: the endpoints and CTA are beta-gated and additive; disabling the CTA / gate removes the
surface with no data cleanup.

## Open Questions

- Scoped-key TTL and exact scope string (resolve during implementation against `api_keys` capabilities).
- Whether the preview pane renders PDF (`GET /:id/pdf`) or a lighter HTML projection for latency.
