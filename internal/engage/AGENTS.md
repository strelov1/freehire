# internal/engage

Reaching the user: notifications on every channel, digests and reminders, onboarding, broadcasts, referrals, community and moderation-adjacent surfaces.

**Layer 7 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate`, `job`, `application`, `search` — and itself.

Must NOT import: `ingest`, `api`.

`ingest` share this layer, and the ban runs both ways: two blocks that can see each other are one block under two names.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`broadcast` `community` `companyfeedback` `emailnotify` `mailpreview` `notify` `nudge` `onboarding` `pushnotify` `referral` `reminder` `report` `subscription` `telegramnotify` `vote`
