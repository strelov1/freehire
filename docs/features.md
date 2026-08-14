# Features

The [README](../README.md) covers how postings get *in* — the crawlers, the
dedup, the normalization. This covers what is built on top of the catalogue:
finding a role, applying to it, and tracking what happens next.

Two markers appear below. **needs `LLM_*`** means the feature calls an
OpenAI-compatible endpoint and stays disabled in a self-host that has not
configured one, the same way OAuth sign-in stays disabled without provider
credentials. **costs AI credits** means it draws on a per-user points balance —
granted monthly, use-it-or-lose-it, debited per action, and earned back by
contributing a board; an exhausted balance is refused with HTTP 402 rather than
degrading into a worse answer. Everything listed is reachable in production
today; nothing is behind a rollout flag.

## Find

### Faceted search

Full-text search over the catalogue, narrowed by region, work mode, seniority,
specialization, skills and salary. Every facet comes from a curated dictionary
rather than from a model guess: an unrecognised value produces no tag at all
instead of a plausible wrong one. The trade is deliberate — what a filter returns
is right, at the cost of a posting phrasing something unusually falling outside
it — and it is why the dictionaries are worth contributing to.

**Live:** /jobs · **Code:** `internal/search`, `internal/skilltag`, `internal/classify`, `internal/location` · **Deep dive:** [internal/search/AGENTS.md](../internal/search/AGENTS.md)

### Collections and role landings

Curated entry points into the catalogue — a stack, a role family, a hiring
profile — each a real filtered view rather than a static page, so a collection
never goes stale against the index behind it.

**Live:** /collections · **Code:** `web/src/lib/collections.ts` (the definitions), `internal/roletag`, `internal/search` (the scoped feed)

### Saved searches, shared boards and digests

A search can be saved, subscribed to, and shared. A subscription mails or
Telegrams a digest of what is new since the last one; a shared board is a
link-addressed, unindexed view of a saved search that you can hand to someone
else.

**Live:** /my/notifications/searches, /b/<slug> · **Code:** `internal/savedsearch`, `internal/subscription`, `internal/notify`, `internal/telegramnotify` · **Deep dive:** [docs/agents/notifications.md](agents/notifications.md)

### Market analytics

Aggregates over the whole catalogue rather than over one search: which skills
are gaining postings, what a role pays by region, which companies are actually
hiring. Built from periodic rollups, so the pages stay fast at catalogue scale.

**Live:** /insights, /trends · **Code:** `internal/facetsnapshot`, `cmd/rollup-facets`, `cmd/rollup-stats`

### The ghost-job signal

Some postings stay open without being filled. freehire flags the behaviour it
can observe — repost patterns, age, how a posting moves — and carries the
evidence alongside the flag. When there is nothing to say it says nothing: this
is a signal with its workings shown, never a verdict about an employer.

**Live:** /features/ghost-jobs · **Code:** `internal/ghost`, `internal/jobreality`, `internal/ghostreport`

## Apply

### CV builder

Structured CVs rendered to PDF through Typst templates, each marked for whether
it survives an ATS parser and whether it prints a headshot. Editing is
revisioned and undoable. Contact details can be masked on a per-CV basis.

**Live:** /my/cvs · **Code:** `internal/cv`, `internal/cvsection`, `internal/headshot`, `internal/pii` · **Deep dive:** [internal/cv/AGENTS.md](../internal/cv/AGENTS.md)

### Match score and fit analysis

Two layers against one vacancy. The deterministic score compares your CV to the
posting's requirements and refuses to credit anything it cannot verify from the
CV. On top of it, an optional LLM pass argues the fit in three stages and
streams its verdict as it goes.

The score itself is free; the analysis on top of it is metered.

**Live:** /match/<slug> · **Code:** `internal/cvmatch`, `internal/jobmatch`, `internal/matchanalysis`, `internal/hardconstraint` · **Deep dive:** [internal/cvmatch/AGENTS.md](../internal/cvmatch/AGENTS.md) — the analysis needs `LLM_*` and costs AI credits

### CV tailoring

Rewrites one CV against one vacancy, requirement by requirement, and invents
nothing: every edit must be backed by an atom in the experience bank, and that
check lives in the service path rather than in a prompt. Autopilot walks the
whole vacancy in a single pass and snapshots the CV first, so the run is
undoable.

**Live:** /tailor/<slug> · **Code:** `internal/cvedit`, `internal/assistant`, `internal/credits` · **Deep dive:** [internal/cvedit/AGENTS.md](../internal/cvedit/AGENTS.md) — needs `LLM_*` and costs AI credits

### The experience bank

Durable employments plus the evidence atoms under them, each recording who
asserted it — you, or the model. Only what you asserted may be written into a
CV, which is what makes tailoring safe to run unattended.

**Live:** /my/profile · **Code:** `internal/experience`, `internal/resumeextract` · **Deep dive:** [internal/experience/AGENTS.md](../internal/experience/AGENTS.md)

### Tracer links

Opt-in link tracing swaps the *target* of the links your CV prints while leaving
the text you wrote intact, so you learn that a PDF was opened and which link was
followed. Consent is per-CV and off until you say otherwise. A deployment with no
visitor salt configured refuses to turn it *on* — recording less while accepting
the consent would answer a question you did not ask — but withdrawing consent is
never refused.

**Live:** /my/cvs/<id>, server:/cv/:token · **Code:** `internal/tracerlink`

### Referrals

A warm intro beats a cold apply. Members already inside a company can offer
referrals, and an opening can be matched to someone able to make one.

**Live:** /referrals · **Code:** `internal/referral`

## Track

### The application board

Every job you view, save, apply to or track lands on a board with stages you
move it through. An application is a durable record of its own: when a posting is
hard-deleted, `cmd/prune` clears the reference and leaves the application
standing, so applying to a role that later vanishes still leaves you a history.
(Plain views and saves are tied to the posting and go with it.)

**Live:** /my/tracking · **Code:** `internal/jobtracking`, `internal/userjob` · **Deep dive:** [internal/userjob/AGENTS.md](../internal/userjob/AGENTS.md)

### The mail inbox

Connect Gmail or use a hosted address, and recruiter replies are classified,
linked to the application they answer, and advance its stage on their own.
Mislinked mail can be relinked by hand, and that undone.

**Live:** /my/inbox · **Code:** `internal/inbox`, `internal/mailingest`, `internal/mailclassify`, `internal/maillink`, `internal/gmailsync`, `internal/mailbox` · **Deep dive:** [docs/agents/mail-stack.md](agents/mail-stack.md)

### The event ledger and reminders

One append-only history of what happened to each application — sent, answered,
advanced, gone quiet — and reminders when a saved job is about to age out or an
application has been silent too long. Notifications go to email or Telegram.

**Live:** /my/activity · **Code:** `internal/appevent`, `internal/reminder`, `internal/emailnotify` · **Deep dive:** [docs/agents/notifications.md](agents/notifications.md)

## Ask

### The in-app assistant

A tool-calling agent that runs inside the API process, streamed over SSE, open to
every signed-in user. There is no shell and no minted credential: a tool receives
the session owner's id and calls a Go service directly, so anything the agent
must not reach is simply a tool that does not exist. Five presets shape one loop
— general chat, browsing the catalogue, filling out your profile, tailoring a
CV, and interview rehearsal. The composer takes dictation as well as typing.

**Live:** /my/assistant/<id> · **Code:** `internal/assistant`, `internal/speech` · **Deep dive:** [internal/assistant/AGENTS.md](../internal/assistant/AGENTS.md) — needs `LLM_*`

### Interview rehearsal

The `interview` preset runs a mock interview against one specific vacancy,
drawing its questions from the gap between that posting's requirements and what
your experience bank already holds — and from the employer's own invitation when
it is sitting in your inbox. A rehearsal binds to a vacancy you have actually
applied to, and the server verifies that binding rather than taking the client's
word for it.

**Live:** /my/assistant/<id> · **Code:** `internal/assistant`, `internal/experience` — needs `LLM_*`

## Build on it

### The public API

The catalogue is served over a keyless HTTP API — no credential, no quota
handshake. Per-user surfaces authenticate with a session cookie or an API key,
and the split is deliberate: the CV builder, the inbox and subscriptions are
cookie-only, so a leaked key cannot act on them.

**Live:** /docs/api · **Code:** `internal/handler`, `internal/auth` · **Deep dive:** [internal/handler/AGENTS.md](../internal/handler/AGENTS.md)

### CLI, browser extension and ChatGPT Actions

The same API drives a CLI, a browser extension that autofills application forms
from your canonical profile, and a ChatGPT Actions manifest. The extension can
also hand its browser to an agent over a relay that routes strictly within one
user's channel.

**Live:** /cli, /chatgpt · **Code:** `internal/browsertools`, `internal/autofillagent` · **Deep dive:** [internal/browsertools/AGENTS.md](../internal/browsertools/AGENTS.md)

### Contributing a board or a posting

Paste a careers URL and freehire works out which ATS it runs on and whether that
board is already covered; a single posting can be submitted directly. Both feed
the same review queue that onboards new sources.

**Live:** /contribute, /submit · **Code:** `internal/contribution`, `internal/submission`, `internal/boardresolve` · **Deep dive:** [internal/contribution/AGENTS.md](../internal/contribution/AGENTS.md)
