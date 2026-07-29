## Context

`POST /me/cvs/tailor` was written for the CTA on the match page: press it, get a tailored copy. The
workspace it opens is addressed `/tailor/<slug>` — by vacancy — and treats a missing `?cv=` as "no
copy yet", so every reload repeats the create. Production shows the result: three CVs on one
vacancy in half an hour, each with its own conversation, none with any messages except the first.

The autopilot change made this visible rather than causing it. Before, the bootstrap auto-sent a
kickoff, so the new conversation was never empty and a reload read as a restart.

## Goals / Non-Goals

**Goals:**
- Reloading the workspace keeps the CV and the conversation.
- A reload is not a second debit.
- Back behaves as it did.

**Non-Goals:**
- Cleaning up the duplicates already created. They are ordinary tailored CVs, they show in
  `/my/cvs`, and deleting a candidate's CVs is not something a bug fix should do quietly.
- Making the workspace addressable by CV only. The vacancy-addressed URL is what the match CTA and
  every existing link use.
- A per-vacancy uniqueness constraint in the schema. The read-then-create is enough for a
  browser-driven flow, and a UNIQUE index would turn a benign race into a 500.

## Decisions

### Idempotence in the store, not in the handler

`cv.Store.Tailor` already owns "what does a tailored copy start from" — seeding the base, copying
the document. "There is already one" belongs in the same place: the handler would otherwise have to
reproduce the ordering (newest first) and the not-found mapping to ask the question.

### The conversation is reused, not re-minted

The handler reads the CV's bound session and mints one only when there is none. It also verifies
the id still resolves: the binding is text on a CV row, and a conversation deleted from
`/my/assistant` would otherwise leave the workspace pointing at nothing.

### The address is corrected client-side, not by a redirect

The bootstrap answers with the CV id; the page rewrites its own URL with `replaceState`. A server
redirect would have to happen before the page knows the id, and pushing a history entry would make
Back step between two states of the same workspace.

### Not a UNIQUE constraint

Two bootstraps racing (double click) can still both create. A unique index would make the loser a
500 on a page that could simply read the winner's row. The window is small, the outcome is the old
behaviour rather than a new failure, and the read-then-create removes the case people actually hit —
reloading.

## Risks / Trade-offs

- **A candidate who WANTED a second tailored CV for the same vacancy no longer gets one from the
  CTA** → They can copy the CV from `/my/cvs`; nothing in the product asked for two copies of one
  vacancy, and the reported behaviour is the opposite complaint.
- **Two simultaneous bootstraps can still duplicate** → Same as today, and the fix removes the
  repeat case (reload) rather than the race.
- **A stale `?cv=` in a bookmark points at a CV that was deleted** → Already handled: the resume
  path 404s the CV and reports it, as it did before this change.

## Migration Plan

No schema change. Ship with the release; the duplicates already on production stay as they are.
