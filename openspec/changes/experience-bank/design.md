## Context

Experience data lives in three unconnected places today, and none of them
accumulates:

- `userprofile.Profile` — specializations, skills, excluded skills, location
  preferences. This is *targeting* ("what I want"), not evidence ("what I did").
- `resumeextract.Structured` — a read-only parse of the uploaded CV, persisted on
  `users` and stamped with `resume_uploaded_at`. Writes are monotonic
  (`WHERE resume_uploaded_at = $stamp`) and a superseded structure reads as absent.
  It is a cache of a derivation, not a store.
- `cv.Document` — the CV builder's editable document, plus one derived copy per
  tailored vacancy.

The tailoring prompt already performs the collection work: for a `missing_gap`
requirement it must ask the candidate before writing anything. The answer is
persisted in the session transcript and nowhere else, so it is unavailable to the
next session, to the fit analysis, and to the base CV. The agent is already mining
evidence; there is no seam to put it.

Two reference implementations informed the shape. `career-ops` separates
`config/profile.yml` (narrative), `interview-prep/story-bank.md` (STAR stories
scored against a question with no LLM), and `verify-cv-facts.mjs` (a guard that
fails a generated CV carrying metrics absent from the sources) — the fabrication
rule is enforced by tooling, not by instruction. Its `skill-extract.mjs` exists
because three scripts had drifted into three skill vocabularies, so a CV saying
"k8s" failed to satisfy a JD requiring "Kubernetes". `ai-job-search`'s
`01-candidate-profile.md` shows the field set a strong bullet actually needs: role,
company, a one-line company context, the metric, and the stack.

## Goals / Non-Goals

**Goals:**

- One durable, user-owned store for what the candidate has done, additive across
  CV re-uploads and chat sessions.
- Make the honest wall machine-checkable: an agent's inference cannot become a CV
  claim without the candidate asserting it.
- Make each tailoring session cheaper than the last — most requirements answered
  from the bank without a question.
- Make evidence gathered anywhere (any preset, the tailor, the profile page) reach
  every consumer: fit analysis, profile read, CV seeding, tailoring.

**Non-Goals:**

- Replacing the CV builder. `cv.Document` remains a self-contained, hand-editable
  snapshot — it has to be, because users edit it and render it to PDF.
- Absorbing education, languages, certifications, or contacts. Those are stable
  form-shaped data with no accumulation problem; they stay in `Structured` and the
  CV document.
- Absorbing targeting. `userprofile` keeps a separate lifecycle.
- Full STAR depth. Atoms are CV-bullet grade. Interview-prep depth is a later
  layer over the same employments, not nullable fields added speculatively now.
- Semantic retrieval. Deterministic scoring first; `internal/embed` is the noted
  seam.

## Decisions

### The bank owns experience; the CV document stays a composed snapshot

Considered making the bank the sole source of truth with CV documents as pure
projections. Rejected: users hand-edit CVs and render them to PDF, so a document
must be self-contained and stable — a CV that silently changes because the bank
changed is a broken artifact. Considered a delta store holding only what is *not*
in the CV. Rejected: the "in the CV or not" boundary moves on every edit, and every
read would need a join against a moving target.

The bank is durable memory; the CV is a composed artifact. Composition is a copy,
made explicit at seed and at tailor time.

### `Employment` is a first-class entity, not a string on the atom

Atoms arrive from three sources at different times, and without normalization
"RingCentral" gets spelled three ways. More importantly, retrieval ranks evidence
partly by recency, and recency is a property of the job, not of the bullet. One
join buys grouping in the UI and recency in the score.

### Provenance is the gate, and it lives in the service

`provenance` is one of `cv_import`, `stated_in_chat`, `manual`, `agent_inferred`.
The first three may be rendered into a CV bullet; `agent_inferred` may not, until
the user confirms it and it is re-stamped.

Considered a `confirmed bool`. Rejected: it collapses the distinction between "the
user wrote this in their own CV" and "the agent guessed", and it gives no audit
trail. Considered leaving the rule in the system prompt, as today. Rejected: that
is exactly the property the whole change exists to make durable — a rule that only
lives in a prompt is a rule that a long conversation eventually loses.

The agent does not choose `provenance` freely. `experience_add` called in the turn
following a user statement records `stated_in_chat`; anything the model originates
is `agent_inferred`. `cv_edit` rejects content whose only backing atom is
`agent_inferred`, and that rejection is a tested path.

### `resumeextract` becomes an importer; import is additive

On upload, `deriveResumeArtifacts` keeps calling the extractor and now hands the
result to `experience.Import`, which reconciles rather than replaces:

- employments matched case-insensitively on (company, role) — found: fill empty
  fields only, never overwrite what the user has edited; not found: create
- atoms matched on a normalized `claim` — found: skip; not found: create with
  `provenance = cv_import`, `source_ref` = the upload stamp

Import never deletes. A user who uploads a trimmed one-page CV must not lose their
bank. Deletion is a user action through the profile UI, only.

The `resume_structured*` columns stay, but their scope narrows to the sections the
bank does not own: contacts, summary, education, languages, links, total years.
Those are stable, form-shaped data with no accumulation problem, and the existing
stamp-and-compare rule remains correct for them. What changes is that experience no
longer answers to that rule — once it lives in its own table, a slow extraction for
an already-replaced CV is not a lost update, it adds atoms that are still true. The
upload stamp survives on the experience side as `source_ref` provenance rather than
as a read gate.

`Professional` therefore becomes a composition: experience from the bank, the rest
from the structured extraction. A user with an empty structure but a full bank gets
experience and no education, which is the correct degradation — the opposite of
today, where a pending extraction hides everything.

### Retrieval is deterministic, and skills are canonicalized through `skilltag`

Scoring an atom against a requirement: canonical skill-slug intersection (dominant
weight), token overlap on `claim`/`context`, employment recency, and a penalty for
`agent_inferred`. A linear pass over one user's atoms.

Atom skills go through the same `internal/skilltag` dictionary that produces the
`jobs.skills` facet, so a vacancy requirement and a piece of evidence are compared
as slug sets. A second vocabulary would reproduce `career-ops`' k8s/Kubernetes
drift on our own data.

Considered routing atoms through `semantic_outbox` and pgvector. Rejected for now:
a user holds tens to low hundreds of atoms, where an exact linear pass is cheaper
and more predictable than an incremental embedding pipeline with a reconciler. The
seam is open and the pattern is already established in `internal/embed`.

### `get_profile` returns the bank's shape; `experience_search` returns its content

A tool result is persisted in the transcript and replayed into the model's context
on every later turn. `Professional` is tolerable at that cost; a two-hundred-atom
bank replayed each turn would consume the window and defeat `trim`. So
`get_profile` reports employments, atom counts and skill coverage, and content is
fetched per query.

### The experience tools are registered under every preset

The moment a person articulates their experience is not scheduled. A `chat` session
about the market is as likely to surface "I actually ran that migration at Sber" as
a tailoring session. The `profile` preset adds intent, not capability: its system
prompt hunts for thin spots — an employment with no atoms, a `userprofile` skill
with no supporting evidence, an achievement with no metric — and asks about them.

## Risks / Trade-offs

- **The read-path switch has no fallback** → `matchanalysis` takes the structured
  candidate profile as its *sole* context; a bank that is empty for a user means no
  analysis. Mitigation: the switch is a separate stage, shipped only after the
  backfill has run and been verified, and the drop of `resume_structured*` is a
  third stage after that.
- **Paraphrase duplicates** → a claim stated in chat and later re-uploaded in a CV
  is matched only if the normalized text agrees. Accepted: two similar atoms are a
  smaller harm than a lost one, and the UI allows deletion. Dedup by similarity is
  deferred until real volumes exist.
- **Bank growth degrades retrieval** → hundreds of atoms make `experience_search`
  noisy. Mitigation: hard top-N in the tool result plus user-driven deletion.
- **`agent_inferred` as an escape hatch** → a model that stamps its own inventions
  as `stated_in_chat` defeats the wall. Mitigation: the gate is in `cv_edit`'s
  service path with a dedicated test; provenance is derived from turn position, not
  taken from the model's argument.
- **Backfill cost** → re-extracting every stored CV through the LLM is the same
  class of spend that has bitten enrichment before. Mitigation: the backfill reuses
  an existing fresh `resume_structured` and only calls the model where none exists.

## Migration Plan

Four stages, each independently shippable:

1. **Additive.** `internal/experience` + migration `0047` + import wired into
   `deriveResumeArtifacts` + `cmd/backfill-experience`. Nothing reads the bank; no
   existing behavior changes. `0047` is applied to prod before this deploy, the
   backfill is run after it.
2. **Read switch.** `Professional` is composed — experience from the bank,
   education and languages from `Structured`; `matchanalysis`, `GET /me/profile`,
   `GET /me/resume` and `cv.Seed` move over.
3. **Agent.** `experience_*` tools across all presets, the `profile` preset, the
   `cv_edit` provenance gate, tailor prompt revision.
4. **UI.** The bank tab under `/my/profile` (review, edit, delete) and the entry
   point into the `profile` preset.

Rollback: stage 1 is inert and can be left in place. Stage 2 is the only risky
deploy, and it is risky only in one direction — a user whose bank failed to seed
loses their fit analysis until it does. No migration destroys data at any stage, so
reverting the projection restores the previous behavior exactly.

## Open Questions

None blocking. Deferred by decision, not by uncertainty: STAR depth over atoms,
similarity-based deduplication, semantic retrieval, and a bullet→atom link (which
would enable a per-atom "used in N CVs" quality signal).
