## Why

Everything the product learns about what a candidate has actually done is either
discarded or overwritten. The tailoring agent is instructed to ask "Do you know X?
Where did you use it?" before writing a gap requirement — and the answer survives
only inside that session's transcript, so the next vacancy asks the same question
again. The one durable store of experience, `resume_structured`, is a cache of the
uploaded file: a re-upload wipes it, and nothing can be added to it. After ten
tailoring sessions a user owns ten derived documents, each holding a fragment of
the truth, and no place where it is assembled.

## What Changes

- **New `internal/experience` package — the experience bank.** Two owner-scoped
  entities: `Employment` (a job or project) and `Atom` (one piece of evidence:
  claim, context, metrics, canonical skill slugs, provenance). The bank is durable
  memory; nothing but the user deletes from it.
- **Provenance replaces prompt discipline as the honest wall.** Every atom records
  where it came from (`cv_import`, `stated_in_chat`, `manual`, `agent_inferred`).
  Only the first three may become a CV bullet; an agent's own inference is a
  legal thing to record and an illegal thing to publish. The check lives in the
  service, not in the system prompt.
- **BREAKING — `resumeextract` stops owning experience.** The parse of an uploaded
  CV becomes a proposal reconciled into the bank (employments matched on
  company+role, atoms on normalized claim; additive, never destructive). The
  `resume_structured*` columns stay, but as the parse cache for the sections the
  bank does not own — contacts, summary, education, languages, links, total years.
- **BREAKING — the `Professional` projection is composed, not read.** Experience
  comes from the bank; education and languages continue to come from the structured
  extraction. Its two consumers, `matchanalysis` (the sole candidate context, no
  fallback) and `GET /me/profile`'s `cv` block, begin to see experience the user
  confirmed in chat and that never appeared in any PDF.
- **The tailoring loop gains a retrieval step and a write-back.** Per requirement:
  search the bank first, surface a matching atom as a reframed bullet; only ask the
  user when the bank is empty on that point, and persist what they answer before
  editing the CV. Tailored documents stay derived and disposable; the knowledge
  they produce does not.
- **Assistant tools `experience_search` / `experience_add` / `experience_update` /
  `experience_employments`, registered under every preset**, because the moment a
  person articulates their experience is not scheduled in advance.
- **New `profile` preset** — an interviewer that finds thin spots in the bank (a
  job with no atoms, a profile skill with no evidence, an achievement with no
  metric) and fills them by asking. Entered from `/my/profile`.
- **A CV is seeded from the bank, not from the extracted résumé**, closing the
  loop: evidence confirmed while tailoring for vacancy #3 lands in the base CV the
  user creates next.
- **`cmd/backfill-experience`** — a run-once worker that seeds the bank for existing
  users, reusing an already-computed `resume_structured` where one is present and
  re-extracting from the stored CV file only where it is not.

## Capabilities

### New Capabilities
- `experience-bank`: the durable per-user store of employments and evidence atoms —
  its shape, its provenance rule, import reconciliation, retrieval, the assistant
  tools that read and write it, the `profile` preset, and the profile-page surface
  for reviewing and deleting what has been recorded.

### Modified Capabilities
- `resume-structured-profile`: extraction stops being a read-only snapshot tied to
  the current résumé and becomes an additive import into the bank; the staleness
  and monotonic-write rules that existed to protect a cache no longer apply to
  experience.
- `cv-tailoring`: the agent consults the bank before asking, persists what the
  candidate confirms, and `cv_edit` refuses content backed only by an
  `agent_inferred` atom.
- `cv-builder`: "Create a CV seeded from the stored résumé" becomes seeded from the
  experience bank.
- `job-fit-analysis`: "Fit is scored from the de-identified structured résumé"
  becomes scored from the de-identified bank projection.
- `assistant-agent-runtime`: a third preset (`profile`) joins `chat` and `tailor`,
  and the experience tools are registered across all presets.

## Impact

**New:** `internal/experience` (domain, store, import, retrieval),
`cmd/backfill-experience`, migration `0047_experience_bank.sql`,
`internal/handler/assistant_experience_tools.go`, a bank tab and preset entry point
under `/my/profile` in `web/`.

**Modified:** `internal/resumeextract` (importer, not store),
`internal/handler` (`deriveResumeArtifacts`, profile/résumé read surfaces,
`assistant_profile_tool.go`, `assistant_cv_tools.go`), `internal/cv` (`seed.go`),
`internal/matchanalysis` (input source), `internal/assistant/prompt.go` (tailor
prompt, new profile prompt), `internal/db/queries` + generated `internal/db`.

**Dependencies:** `internal/skilltag` becomes a write-path dependency of the bank —
atom skills are canonicalized through the same dictionary that produces the
`jobs.skills` facet, so "does the candidate have evidence for this requirement" is
a set intersection rather than a text-matching problem.

**Operational:** migration `0047` must be applied to prod before the deploy that
reads it; the backfill runs once after; the projection switch ships only once the
backfill is verified, because `matchanalysis` has no raw-CV fallback.
