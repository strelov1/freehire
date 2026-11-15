# Repository routing map

Read the applicable nested AGENTS.md before editing. Before proposing new work, read
[.out-of-scope/](.out-of-scope/README.md); follow an existing decision rather than reinvestigating it.
Use surgical, idiomatic changes; no placeholders or speculative infrastructure. English only.

Before implementation, read the relevant sections of [repository requirements](AGENT-REFERENCE.md):
API/auth/paid AI changes require Conventions; SQL changes require Layout and Commands;
public counts, attribution and logos require Conventions and docs/agents/public-figures.md.
Cross-module changes require Layout and docs/knowledge/index.md; update knowledge provenance.
UI changes require web/AGENTS.md and design-system/AGENTS.md; worker changes require
 docs/agents/workers.md before running or editing the worker.

Never edit applied migrations or generated DB/API code: change SQL queries or TypeSpec inputs
and regenerate. Preserve paid AI middleware and context binding, authenticated caller ownership,
email takeover defenses, and snapshot-only public figures. Read their full requirements before touching them.

Before committing Go: gofmt, go vet ./..., go test ./.... Before every push:
go vet -tags=integration ./.... Keep existing check commands in Commands below.
PR titles use Conventional Commits. Host configuration belongs in freehire-ops and needs host application too.

For an explained local check plan, run `python3 scripts/check-local.py --base origin/main`;
add `--run` to execute it. Local results do not replace behavior integration tests or CI.

For scoped source searches use `python3 scripts/source-search.py SCOPE PATTERN`.
Use `--full` for history, generated files and other normally omitted paths. SQL queries,
migrations and generation inputs remain searchable. See [search usage](docs/agents/source-search.md).


## Working principles

Read [Working principles](AGENT-REFERENCE.md#working-principles) before work involving this topic.

## What this is

Read [What this is](AGENT-REFERENCE.md#what-this-is) before work involving this topic.

## Layout

Read [Layout](AGENT-REFERENCE.md#layout) before work involving this topic.

## Commands

Read [Commands](AGENT-REFERENCE.md#commands) before work involving this topic.

## Knowledge navigation

Read [Knowledge navigation](AGENT-REFERENCE.md#knowledge-navigation) before work involving this topic.

## Module files

Each is self-contained and can be read independently.

| Area | Reference |
|---|---|
| **System architecture** — start here | [docs/architecture.md](docs/architecture.md) |
| **CI gates and linters** | [docs/agents/ci-checks.md](docs/agents/ci-checks.md) |
| **Public figures** (catalogue/source stats, logos, sign-up attribution) | [docs/agents/public-figures.md](docs/agents/public-figures.md) |
| **Browser extension** | [extension/AGENTS.md](extension/AGENTS.md) |
| **Company identity** (slug rule, aliases, merge) | [docs/agents/company-identity.md](docs/agents/company-identity.md) |
| **Mail stack** (inbound → classify → link → stage) | [docs/agents/mail-stack.md](docs/agents/mail-stack.md) |
| **Notifications** (digests, reminders, channels) | [docs/agents/notifications.md](docs/agents/notifications.md) |
| **Job lifecycle** (soft-close, sweep, liveness) | [docs/agents/job-lifecycle.md](docs/agents/job-lifecycle.md) |
| **Worker commands** (each `cmd/<name>`) | [docs/agents/workers.md](docs/agents/workers.md) |
| **Company facets** (remote_regions, yc_*) | [docs/agents/company-facets.md](docs/agents/company-facets.md) |
| **SPA** (SvelteKit, auth flow, API conventions) | [web/AGENTS.md](web/AGENTS.md) |
| **Host configuration** — NOT in this repo | the private `freehire-ops`: `provision/host2/AGENTS.md` |
| **Design system** (tokens, Tailwind @source, shared deps) | [design-system/AGENTS.md](design-system/AGENTS.md) |
| **`internal/platform`** block | [internal/platform/AGENTS.md](internal/platform/AGENTS.md) |
| **SQL layer** (sqlc, queries, migrations) | [internal/platform/db/AGENTS.md](internal/platform/db/AGENTS.md) |
| **Cron worker plumbing** | [internal/platform/worker/AGENTS.md](internal/platform/worker/AGENTS.md) |
| **LLM client** | [internal/platform/llm/AGENTS.md](internal/platform/llm/AGENTS.md) |
| **Headless browser** | [internal/platform/browser/AGENTS.md](internal/platform/browser/AGENTS.md) |
| **Scrapling fetch tier** | [internal/platform/scraplingfetch/AGENTS.md](internal/platform/scraplingfetch/AGENTS.md) |
| **Exception logs and metrics** (OpenTelemetry, Prometheus) | [internal/platform/observability/AGENTS.md](internal/platform/observability/AGENTS.md) |
| **`internal/dict`** block | [internal/dict/AGENTS.md](internal/dict/AGENTS.md) |
| **Company names** | [internal/dict/companyname/AGENTS.md](internal/dict/companyname/AGENTS.md) |
| **Enrichment** (contract, provider) | [internal/ai/enrich/AGENTS.md](internal/ai/enrich/AGENTS.md) |
| **Geography** | [internal/dict/location/AGENTS.md](internal/dict/location/AGENTS.md) |
| **Skill tagging** | [internal/dict/skilltag/AGENTS.md](internal/dict/skilltag/AGENTS.md) |
| **Answer topics** | [internal/dict/answertopic/AGENTS.md](internal/dict/answertopic/AGENTS.md) |
| **Skill vectors** | [internal/dict/skillvec/AGENTS.md](internal/dict/skillvec/AGENTS.md) |
| **Seniority & category** | [internal/dict/classify/AGENTS.md](internal/dict/classify/AGENTS.md) |
| **`internal/ai`** block | [internal/ai/AGENTS.md](internal/ai/AGENTS.md) |
| **Browser tools** (harness ↔ extension relay) | [internal/ai/browsertools/AGENTS.md](internal/ai/browsertools/AGENTS.md) |
| **Semantic embedding** | [internal/ai/embed/AGENTS.md](internal/ai/embed/AGENTS.md) |
| **In-app assistant** | [internal/ai/assistant/AGENTS.md](internal/ai/assistant/AGENTS.md) |
| **Speech to text** | [internal/ai/speech/AGENTS.md](internal/ai/speech/AGENTS.md) |
| **LLM spend attribution** | [internal/ai/llmkey/AGENTS.md](internal/ai/llmkey/AGENTS.md) |
| **Plan limits and the paid gate** | [internal/ai/plan/AGENTS.md](internal/ai/plan/AGENTS.md) |
| **`internal/identity`** block | [internal/identity/AGENTS.md](internal/identity/AGENTS.md) |
| **Accounts** | [internal/identity/accounts/AGENTS.md](internal/identity/accounts/AGENTS.md) |
| **Auth primitives** | [internal/identity/auth/AGENTS.md](internal/identity/auth/AGENTS.md) |
| **OAuth sign-in** | [internal/identity/auth/oauth/AGENTS.md](internal/identity/auth/oauth/AGENTS.md) |
| **Discounts** (promo codes, invites) | [internal/identity/promo/AGENTS.md](internal/identity/promo/AGENTS.md) |
| **`internal/candidate`** block | [internal/candidate/AGENTS.md](internal/candidate/AGENTS.md) |
| **AI fit analysis** (prompt chain) | [internal/candidate/matchanalysis/AGENTS.md](internal/candidate/matchanalysis/AGENTS.md) |
| **Fit-analysis use cases** (cache, credit) | [internal/candidate/fitanalysis/AGENTS.md](internal/candidate/fitanalysis/AGENTS.md) |
| **Job-match scoring** | [internal/candidate/cvmatch/AGENTS.md](internal/candidate/cvmatch/AGENTS.md) |
| **ATS-readiness score** | [internal/candidate/atscheck/AGENTS.md](internal/candidate/atscheck/AGENTS.md) |
| **Experience bank** | [internal/candidate/experience/AGENTS.md](internal/candidate/experience/AGENTS.md) |
| **Screening answer bank** | [internal/candidate/answerbank/AGENTS.md](internal/candidate/answerbank/AGENTS.md) |
| **Résumé identity** | [internal/candidate/resume/AGENTS.md](internal/candidate/resume/AGENTS.md) |
| **Talent Network** | [internal/candidate/talentnetwork/AGENTS.md](internal/candidate/talentnetwork/AGENTS.md) |
| **Structured CV** | [internal/candidate/resumeextract/AGENTS.md](internal/candidate/resumeextract/AGENTS.md) |
| **PII masking** | [internal/candidate/pii/AGENTS.md](internal/candidate/pii/AGENTS.md) |
| **Cover letters** | [internal/candidate/coverletter/AGENTS.md](internal/candidate/coverletter/AGENTS.md) |
| **CV rendering** | [internal/candidate/cv/AGENTS.md](internal/candidate/cv/AGENTS.md) |
| **CV edits** (the only writer) | [internal/candidate/cvedit/AGENTS.md](internal/candidate/cvedit/AGENTS.md) |
| **`internal/job`** block | [internal/job/AGENTS.md](internal/job/AGENTS.md) |
| **Job wire shape** | [internal/job/jobview/AGENTS.md](internal/job/jobview/AGENTS.md) |
| **Job fingerprints** (which hash for which job) | [internal/job/jobhash/AGENTS.md](internal/job/jobhash/AGENTS.md) |
| **Requirements extraction** | [internal/job/reqextract/AGENTS.md](internal/job/reqextract/AGENTS.md) |
| **Ghost detection** | [internal/job/ghost/AGENTS.md](internal/job/ghost/AGENTS.md) |
| **Search announcements** (Indexing API, IndexNow) | [internal/job/searchping/AGENTS.md](internal/job/searchping/AGENTS.md) |
| **YC directory** | [internal/job/ycdir/AGENTS.md](internal/job/ycdir/AGENTS.md) |
| **Wikidata company matching** | [internal/job/wikicompany/AGENTS.md](internal/job/wikicompany/AGENTS.md) |
| **Company collections** | [internal/job/collections/AGENTS.md](internal/job/collections/AGENTS.md) |
| **`internal/application`** block | [internal/application/AGENTS.md](internal/application/AGENTS.md) |
| **Per-user job tracking** | [internal/application/userjob/AGENTS.md](internal/application/userjob/AGENTS.md) |
| **View counts** | [internal/application/viewlog/AGENTS.md](internal/application/viewlog/AGENTS.md) |
| **Agent views** | [internal/application/agentview/AGENTS.md](internal/application/agentview/AGENTS.md) |
| **Auto-apply queue** | [internal/application/autoapply/AGENTS.md](internal/application/autoapply/AGENTS.md) |
| **`internal/search`** block; **Search** (index topology, reindex, dropped filters) | [internal/search/AGENTS.md](internal/search/AGENTS.md) |
| **Facet-search drain** | [internal/search/searchdrain/AGENTS.md](internal/search/searchdrain/AGENTS.md) |
| **Facet engine** (facetd's in-memory index, filter evaluator, the Meilisearch rules it copies) | [internal/search/facetidx/AGENTS.md](internal/search/facetidx/AGENTS.md) |
| **`internal/ingest`** block | [internal/ingest/AGENTS.md](internal/ingest/AGENTS.md) |
| **Source ingest** (adapters, registry) | [internal/ingest/sources/AGENTS.md](internal/ingest/sources/AGENTS.md) |
| **Ingest scheduling** | [internal/ingest/ingestsched/AGENTS.md](internal/ingest/ingestsched/AGENTS.md) |
| **Board catalog** (`boards`, `cmd/add-board`) | [internal/ingest/boardcatalog/AGENTS.md](internal/ingest/boardcatalog/AGENTS.md) |
| **Pipeline** (Runner, dedup, board health) | [internal/ingest/pipeline/AGENTS.md](internal/ingest/pipeline/AGENTS.md) |
| **Apply-form capture** | [internal/ingest/applyform/AGENTS.md](internal/ingest/applyform/AGENTS.md) |
| **Screening answers** | [internal/ingest/screeninganswers/AGENTS.md](internal/ingest/screeninganswers/AGENTS.md) |
| **Link resolution** | [internal/ingest/linksource/AGENTS.md](internal/ingest/linksource/AGENTS.md) |
| **ATS board recognition** | [internal/ingest/atsboard/AGENTS.md](internal/ingest/atsboard/AGENTS.md) |
| **Board contributions** | [internal/ingest/contribution/AGENTS.md](internal/ingest/contribution/AGENTS.md) |
| **Employer accounts** | [internal/ingest/employer/AGENTS.md](internal/ingest/employer/AGENTS.md) |
| **On-site applications** | [internal/ingest/directapply/AGENTS.md](internal/ingest/directapply/AGENTS.md) |
| **Telegram** | [internal/ingest/telegram/AGENTS.md](internal/ingest/telegram/AGENTS.md) |
| **`internal/engage`** block | [internal/engage/AGENTS.md](internal/engage/AGENTS.md) |
| **CV lifecycle letters** | [internal/engage/cvlifecycle/AGENTS.md](internal/engage/cvlifecycle/AGENTS.md) |
| **Interview buddies** | [internal/engage/buddy/AGENTS.md](internal/engage/buddy/AGENTS.md) |
| **Messaging core** | [internal/engage/messaging/AGENTS.md](internal/engage/messaging/AGENTS.md) |
| **Employee referrals** | [internal/engage/referral/AGENTS.md](internal/engage/referral/AGENTS.md) |
| **Mentorship marketplace** | [internal/engage/mentorship/AGENTS.md](internal/engage/mentorship/AGENTS.md) |
| **Daily social digest** | [internal/engage/socialdigest/AGENTS.md](internal/engage/socialdigest/AGENTS.md) |
| **LinkedIn credential** | [internal/engage/linkedinauth/AGENTS.md](internal/engage/linkedinauth/AGENTS.md) |
| **Discord paid channels** | [internal/engage/discordlink/AGENTS.md](internal/engage/discordlink/AGENTS.md) |
| **`internal/api`** block | [internal/api/AGENTS.md](internal/api/AGENTS.md) |
| **HTTP handlers** (responses, errors, routes) | [internal/api/handler/AGENTS.md](internal/api/handler/AGENTS.md) |
| **Auto-apply browser driver** | [internal/api/atsapply/AGENTS.md](internal/api/atsapply/AGENTS.md) |
| **ChatGPT app** (the second MCP server) | [internal/api/mcpapp/AGENTS.md](internal/api/mcpapp/AGENTS.md) |
| **OJCP provider** | [internal/api/ojcp/AGENTS.md](internal/api/ojcp/AGENTS.md) |


Read [Module files](AGENT-REFERENCE.md#module-files) before work involving this topic.

## Conventions

Read [Conventions](AGENT-REFERENCE.md#conventions) before work involving this topic.
