## Why

Reported from production: opening `/tailor/senior-backend-engineer-appinio-slztl4lb` and then
reloading loses the conversation. The data says why — three tailored CVs on that one vacancy
inside half an hour, each with its own empty conversation:

```
8efbd881…  19:38  bound session, 0 messages
59c41575…  19:57  bound session, 0 messages
84738209…  20:05  bound session, 0 messages
```

The workspace's address is `/tailor/<slug>`. Without a `?cv=` reference the page bootstraps, and
`POST /me/cvs/tailor` has always created a NEW copy — so a reload mints another CV, mints another
conversation, rebinds the CV to it, and whatever the candidate had said stays on the previous one.

This predates the autopilot. It was hidden by the automatic kickoff: a fresh conversation filled
immediately with the agent's opening, so a reload looked like a chat that had restarted rather
than one that had vanished. Removing the self-start made the existing bug visible.

## What Changes

- `POST /me/cvs/tailor` becomes idempotent per (user, vacancy): it returns the existing tailored
  CV when there is one, and the conversation already bound to it, instead of minting a second of
  each. A different vacancy still gets its own copy.
- A session id pointing at a conversation that no longer exists counts as no session, so a deleted
  conversation cannot strand the workspace.
- The workspace puts the CV in the address as soon as it has one (`?cv=<id>`, replacing the history
  entry rather than adding one), so a reload lands in the resume path.
- **A reload is not a second purchase** — the tailor cost is charged once per tailored CV, and a
  reload now reaches the same CV.

## Capabilities

### New Capabilities
<!-- none: this fixes existing behaviour -->

### Modified Capabilities
- `cv-tailoring`: the bootstrap reaches one tailored copy per vacancy rather than creating one per
  request.
- `tailor-workspace`: the workspace names its CV in the address, so a reload resumes instead of
  bootstrapping.

## Impact

- **Go:** `internal/db/queries/cvs.sql` (+ regenerated sqlc), `internal/cv/store.go` (`Tailor`),
  `internal/handler/cv_tailor.go` (reuse the bound conversation).
- **Web:** `web/src/routes/tailor/[slug]/+page.svelte` (replace the address after bootstrap).
- **No schema change.** The duplicate CVs already on production are left alone — they are ordinary
  tailored CVs and show up in `/my/cvs`.
