## Context

`internal/candidate/cvedit` already has a precedent for moving a rule the model sometimes misses
out of the system prompt and into the service path, stated in that package's own comments:

> "Two rules the tool no longer states because the editor enforces them... the honest wall made
> structural rather than instructional, in the service path rather than a prompt."

That precedent covers the contact block (`ErrForbiddenPath`) and evidence citation
(`ErrEvidenceRequired`). The Projects-vs-Experience misfile is a third instance of the same class
of problem — prompt wording helps but does not guarantee — and `cv.Project`'s shape (no
company/date fields) gives a precise, cheap signal to check structurally rather than inventing a
heuristic over free text.

## Goals / Non-Goals

**Goals:**
- Make the Projects-vs-Experience placement rule a structural refusal for the one case that is
  both common and detectable with low false-positive risk: a project name carrying a job's
  `<start> - <end>` tenure span.
- Keep the refusal actor-scoped (agent only), matching `requireEvidence`'s own actor check, so a
  candidate typing their own project name through the CLI-edit endpoint is never blocked.
- Give the model a self-correcting error message, the same contract `ErrForbiddenPath` and
  `ErrEvidenceRequired` already honor: the error names the fix ("use `experience[]]` instead").

**Non-Goals:**
- Catching every misfile. A project named "Freelance Consulting, Acme Corp" (no date span) is not
  caught — there is no comparably precise signal for "names a company" without false-positives on
  real client-named portfolio projects (e.g. "Redesign for Acme Corp").
- Checking bullets for date mentions — legitimate project bullets routinely cite a year
  ("reduced P95 latency in 2021") without being misfiled.
- A candidate-facing UI banner. Unlike `ErrListCap` (whose failure is a terminal, visible state —
  "this role already has the maximum bullets" — that benefits from a stable `Code` the SPA can
  key a banner on), this refusal is agent-only and self-correcting within the turn: the candidate
  asked for an edit and, after one extra tool round, gets it filed correctly. There is nothing
  terminal for a banner to explain.
- Any new environment-variable kill switch. `SetRefuseListCap` exists because truncating bullets
  was the OLD, accepted behavior that some caller might still want; there is no comparable "old
  behavior" worth preserving here.

## Decisions

- **Actor-gated, not universal.** `refuseIfProjectLooksLikeJob` only runs when `ch.Actor ==
  ActorAgent`, checked in `commit()` next to the existing `refuseIfSanitizeDropsContent` call.
  `CommitDocument` needs no change: it already refuses `ActorAgent` outright (line ~368), so every
  call through it — the structured editor form, résumé reseed, header heal, surface-align — is
  already guaranteed to be candidate or system, never agent.
- **Check `name` only, not bullets.** `cv.Project.Name` is the one field with no legitimate reason
  to carry a date range; `Bullets` routinely do. Checking only `name` keeps the false-positive rate
  near zero without needing a more elaborate classifier.
- **Whole-batch refusal, not a silent fix-up.** Matches `ErrListCap`/`ErrForbiddenPath`/
  `ErrEvidenceRequired`: a batch either lands whole or not at all, so History never shows a change
  that didn't fully happen. The alternative — silently moving the entry to `experience[]` on the
  model's behalf — was considered and rejected: the editor does not have enough information to
  invent a `role`/`company` split from a free-text project name, and guessing wrong would be a
  worse failure mode than asking the model to redo it with the fields it already has from the
  conversation.
- **Regex, not an LLM call or NLP library.** A second model call to classify "is this a job" would
  cost a round-trip and a prompt budget to decide something a four-character date-range pattern
  already answers with no additional latency or spend.
- **Read each op's own `Value`, never re-derive a position from the final document.** The first
  draft resolved an op's literal path index (e.g. `projects[0]`) against `applied.Projects` —
  the state AFTER every op in the batch ran. That is wrong whenever another op in the same batch
  inserts or removes ahead of it: a `set` on `projects[0].name` and a later `insert` at
  `projects[0]` both carry the path `projects[0]`, but after the insert shifts things, index 0 in
  the final state is the INSERTED entry, not the one the `set` wrote — so the check looked at the
  wrong entry and let a job-shaped name through. Reading the op's own `Value` directly (the
  string for a `.name` path, or the `name` field of a whole-project value) sidesteps position
  entirely: it is exactly what that operation is trying to write, independent of where the list
  repositions it afterward.

## Risks / Trade-offs

- [Risk] A portfolio project legitimately named with a date range (e.g., "Game Jam 2020 - 2021")
  is refused.
  → Mitigation: rare in practice (game jams, hackathons, and similar short events are usually
  named by the event, not a tenure span); the refusal message tells the model exactly what tripped
  it, and the candidate (or model) can still get the exact text into `projects[].bullets` rather
  than `name`, or override by having the candidate edit it directly via their own CLI/editor call
  (`ActorCandidate`, never checked).
- [Risk] The date-range regex misses non-numeric-year phrasings ("early 2020 through last year").
  → Mitigation: accepted. This is a best-effort structural net under the existing prompt guidance,
  not a replacement for it — the two layers together catch more than either alone, and the regex
  is intentionally narrow to keep false positives near zero rather than chasing every phrasing.
