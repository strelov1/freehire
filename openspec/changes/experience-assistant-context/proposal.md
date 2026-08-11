# Proposal: the agent can read the achievements it is asked about

## Why

Four surfaces hand the agent the identifier of an achievement, and nothing lets it read
one.

The experience tools are asymmetric in a way that only shows up in use. `experience_search`
is the sole tool that returns an atom's content, and it retrieves by meaning —
`experience.Query` carries text and skills and nothing else. `experience_update` and
`experience_merge` take ids and only write. `get_profile` reports `soft_duplicate_clusters`
as bare id lists, with a comment that says so out loud: *"id-lists of near-paraphrase
achievements (no claim text)"*. So the agent is told which achievements are near-duplicates
and cannot see any of them.

The in-place panel made this visible. Selecting four achievements sends an opening message
naming their ids; the agent has no tool that takes an id, so it passed them to
`experience_search` as query text. A UUID overlaps no atom's words, zero-scoring matches
are deliberately dropped, and the tool's own description tells the model what an empty
result means: *"the bank holds nothing on that point, so ask them."* The agent was
therefore told, four times, that the achievements the candidate had just selected did not
exist. It searched more and more broadly, then answered about a different set of
achievements entirely — the ones a text search happened to surface.

The capability is already there and simply unreachable: `GetAtom(ctx, id, userID)` sits on
the `experienceBankTools` interface and is called inside `experience_update` to load an
atom before patching it.

The consequences split in a way worth naming, because they are not equally bad. **Merging
works blind**, which is the hazard rather than the bug: `MergeAtoms` is deterministic
server-side — it keeps the richer atom, unions metrics and skills, keeps the better
context — so the agent can merge two achievements it has never read, and cannot make the
one judgement a merge needs. **Refining does not work at all**: `experience_update`'s
fields replace what is stored, so sharpening an atom without first reading it overwrites
whatever was there.

Separately, the panel is laid out two flex rows inside the account shell's content column,
so it pays the `max-w-6xl` cap twice. After the nav and the gaps, the panel and the bank
split roughly 860px between them and both end up cramped — the conversation is hard to
read and the bank it is about is harder.

## What Changes

- The agent gains a **read-by-id tool** for the experience bank, wrapping the `GetAtom`
  the service already exposes. It accepts several ids at once, is capped the way
  `experience_search` is capped, and returns the same atom shape a search returns, so the
  model sees one consistent achievement whichever tool produced it.
- An id the caller does not own, or that does not exist, is **named back rather than
  failing the call**. A partial answer is the useful one — the same choice
  `resolveEmployment` already makes when it refuses a bad `employment_id` by listing the
  ids that would have worked.
- The `profile` preset's prompt and the interviewer's opening message **tell the agent to
  read the named achievements before proposing anything**. The tool alone does not fix the
  behaviour: an agent that can merge blind will still merge blind unless it is told to look
  first.
- The in-place panel **leaves the account shell's content column** and docks against the
  viewport, so it no longer spends the bank's width. Because the shell is centred and
  capped, this means the shell yields while the panel is open rather than the panel
  appearing in margin that is not there on a laptop.
- `get_profile` keeps reporting duplicate clusters as ids without claim text. That was
  always the right call — a tool result is replayed into the model's context every
  later turn — and it only became a dead end because nothing could resolve the ids. The
  summary stays shape, the content becomes fetchable.

### Non-goals

- **The agent does not get to author merged claim text.** Merge keeps text the candidate
  wrote, and that is load-bearing: letting the model rewrite a claim during a merge would
  launder `agent_inferred` prose into a publishable atom straight past the evidence gate.
  Noted here so it stays a decision rather than becoming an accident.
- **The panel does not become the account shell's dock.** Hosting it in `my/+layout.svelte`
  so Tracking, Inbox and CVs could open it needs a per-page launch contract that nothing is
  asking for yet. The seam is real; this change is not the place to take it.

## Capabilities

### New Capabilities

None. The bank, its agent surface and the panel all exist; this closes a gap in what the
agent can read and moves where the panel sits.

### Modified Capabilities

- `experience-bank`: the agent gains the ability to read specific achievements by id, is
  required to read them before acting on them, and the in-place panel stops competing with
  the bank for width.

## Impact

Backend and frontend, no schema or worker change and no new HTTP endpoint — the new tool
reads through the store the existing tools already use.

- `internal/handler/assistant_experience_tools.go` — the read tool, registered alongside
  the others; the shared atom-result shape currently private to `searchResult`.
- `internal/assistant/prompt.go` — the `profile` preset's instruction to read before
  proposing.
- `web/src/lib/assistant/presets.ts` — the opening message says to read the named
  achievements first.
- `web/src/lib/components/ExperienceAssistantPanel.svelte` and
  `ExperienceBankView.svelte` — the panel leaves the content column.
- `web/src/routes/my/+layout.svelte` — the shell yields while the panel is open.
- `internal/assistant/AGENTS.md` and `web/AGENTS.md` — record the read tool and the
  panel's placement.

The risk worth watching is the layout one: the panel now reaches outside the component
that owns it to offset a shell it does not own, which is the kind of coupling that rots.
The design pins down how that offset is expressed.
