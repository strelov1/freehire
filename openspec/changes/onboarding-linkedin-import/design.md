## Context

The `/onboarding` wizard stages role, skills, level and location locally across three steps and commits them in one `PUT /api/v1/me/profile` when the user leaves. Step 1 (CV) is the only thing that pre-fills the rest: a PDF or pasted text goes to `POST /api/v1/me/resume/extract`, which runs `resumeProfile(text)` (`internal/api/handler/resume.go:192`) — `skilltag.Parse` for skills, `classify.Categories`/`classify.Parse` over the résumé headline for category and seniority — and returns `{skills, categories, seniority?}`. A user with no PDF gets none of that.

The feasibility spike (2026-09-03) settled what a public LinkedIn profile gives an anonymous fetch. The page returns 200 with one `application/ld+json` block containing a `Person` node, but most text arrives as runs of asterisks that preserve the original length — 479 such strings, covering every `jobTitle` entry and every position description. Unmasked: `description` (the headline, truncated with an ellipsis), `address`, `knowsLanguage`, `image`, `name`, the first `worksFor` entry's company name and location, and the first `alumniOf` entry. A Googlebot user-agent, a country subdomain, an explicit locale and the `/details/experience/` path each return the identical 479 masks.

The headline is the whole opportunity. It is conventionally "level + role + stack", which is the same shape a résumé headline has, and the existing dictionaries handle it unchanged. Measured on the spike's real headline (`Senior Backend Engineer working in TypeScript/Node.js, Go, and Python, with, focused on…`): `classify.Parse` → `{Seniority: senior, Category: backend}`, `skilltag.Parse` → `[nodejs python typescript]`, and `location.Parse("Florianópolis, Santa Catarina, Brazil")` → `{Countries: [br], Regions: [latam], Cities: [Florianópolis Santa Catarina]}`.

## Goals / Non-Goals

**Goals:**

- A user with no CV file can pre-fill the wizard's four fields from a URL they can always produce.
- The derived values are *identical* to what the CV path would derive from the same text — one vocabulary, not two.
- The user is told, before they try it, that work history is not part of what LinkedIn releases here.
- A failed or fruitless import costs nothing: the step stays exactly as it was.

**Non-Goals:**

- Employment history, the experience bank, education records. The source data is masked; this is settled, not deferred-because-hard.
- Any authenticated read of LinkedIn. No cookie is accepted, sent, or stored — not the caller's, not a service account's.
- Persisting the fetched page, or any of it, outside the response to the request that asked for it.
- Reading the profile through `extension/` with the user's own session. That would yield the full history and is the natural follow-up, but it is a separate change with a separate delivery surface.

## Decisions

### The package fetches and unmasks; the handler derives

`internal/candidate/linkedinprofile` is responsible for exactly two things: getting the public page and turning its `ld+json` `Person` node into a struct of *unmasked* raw strings (`Name`, `Headline`, `Location`, `Company`, `Languages`). It runs no dictionary and reaches no vocabulary.

The handler then calls the **existing** `resumeProfile(text)` with the headline, and `location.Parse` with the address.

**Why:** the response must carry the same `{skills, categories, seniority}` the CV path produces, and the only way to guarantee that as both evolve is to call the same function. A second derivation in the new package would be a second answer to the same question, and the drift between them would look exactly like a dictionary bug. This also keeps `linkedinprofile` dependent on nothing but `platform/safehttp` and the standard library.

**Alternative rejected:** deriving inside `linkedinprofile` and returning finished facets. Reads better in isolation, duplicates the vocabulary, and would need `dict` imports the package otherwise does not want.

### Masked values are dropped at the parse boundary, not downstream

A value is treated as absent when, with separators removed, nothing but asterisks remains. The check runs once, inside `linkedinprofile`, on every string it lifts out of the node — so no masked string can reach a dictionary, a response field, or the user's staged profile.

**Why:** this is the failure mode with real consequences. `worksFor[0].name` happened to be unmasked on the profile tested, but the second entry was `"**********"`; a naive read writes that into a profile as a company name. Putting the check at the boundary makes it structurally impossible rather than a rule every call site must remember.

**Alternative rejected:** filtering in the handler. Leaves the package able to emit masked strings, so every future caller re-inherits the trap.

### Anonymous fetch, host allowlist, byte cap, guarded client

The fetch carries no credentials, goes out through `safehttp`'s guarded client (SSRF protection on redirects), enforces a timeout, and reads at most a fixed cap of the body. The URL is validated against a member-profile shape (`linkedin.com` / `www.` / `<cc>.` host, `/in/<public-id>` path) **before** any outbound request, and a bare public id is accepted and expanded.

**Why:** this is the first user-supplied third-party URL that a `/me` handler will fetch, so it is an SSRF surface by construction. The measured page is ~600 KB, so an unbounded `io.ReadAll` on an attacker-influenced response is not acceptable; the cap turns a hostile response into a failed import. Validating before fetching means a rejected URL costs no outbound request at all.

**Alternative rejected:** fetching from the browser. CORS blocks reading `linkedin.com` cross-origin, and the dictionaries are server-side regardless.

### The endpoint returns the CV path's shape plus location, and stores nothing

`POST /api/v1/me/linkedin/import` returns `{skills, categories, seniority?}` — byte-identical in shape to `/me/resume/extract` — plus the derived location and the display fields (name, headline, company) the UI shows as "here is what we recognised".

Notably it does **not** reuse the `/me/resume/extract` *handler*, which also stores the file and derives résumé artifacts when storage is configured. An import is not a CV: it must not set CV presence, because CV presence is the wizard's own redirect gate, and an import that silently satisfied the gate would stop prompting a user who still has no CV.

**Why the same shape anyway:** the client merges both sources into one staged set. Two shapes would mean two merge paths.

### The import is a second entry point on the CV step, not a fourth step

The step keeps its dropzone and gains a URL field beside it, plus a permanent disclosure naming LinkedIn's `More → Save to PDF` export. That export is a PDF, and the dropzone already accepts PDFs (`accept=".pdf,application/pdf"`), so the honest fallback for a user who wants their history in costs no new code path.

**Why:** the wizard's step count and skip semantics are specified behaviour in `onboarding-cv-wizard`; adding a step would change them. Adding a source to an existing step does not.

### Third-party enrichment vendors were priced and rejected

The spike found paid endpoints that return full LinkedIn profiles at roughly $0.03–$0.15 per profile.

**Rejected because:** they resell data obtained the same way this design declines to obtain it, they put a per-user cost on a free onboarding step, and they add a vendor whose supply can vanish on one legal letter — the standalone tools in this space have a history of exactly that. The masked-headline path has no vendor and no per-call cost.

## Risks / Trade-offs

- **LinkedIn changes the `ld+json` shape or drops the `Person` node** → the import returns "could not read this profile" and the step stays usable. The spec requires graceful failure, so this degrades rather than breaks. Parsing is one small function with the failure path already specified.
- **LinkedIn serves an authwall or a challenge to the production IP** → same graceful failure, but silently, for everyone. Mitigation: the failure must be distinguishable in logs from a bad URL, so a sustained drop is visible rather than looking like users pasting nonsense.
- **The headline resolves nothing** (a non-English headline, or "Helping companies grow") → a successful import with empty values, which the UI must report as "couldn't read details" rather than as an error. This is the same wording the CV step already uses when extraction resolves nothing.
- **Short skill tokens are missed** — the spike's headline mentioned Go and `skilltag.Parse` did not tag it, a known short-token gap in the dictionary. Trade-off accepted: the confirm step is editable, and the alternative is fixing word-boundary handling, which is a different change with a much wider blast radius.
- **Terms of service.** The fetch is anonymous, user-initiated, one profile at a time, rate limited, unauthenticated, and nothing is stored. That is a materially different posture from credentialed bulk scraping, but it is not zero risk, and the rate limit is what keeps it from drifting into volume.
- **An outbound fetch on a request path** adds third-party latency to a `/me` endpoint. Bounded by the timeout; the UI shows a pending state as the dropzone already does for extraction.

## Migration Plan

No database changes and no migration. The package, the endpoint and the UI ship together; the endpoint is inert until the UI calls it. Rollback is removing the UI entry point — the endpoint left in place is unreachable and harmless.

The new package must be added to the `candidate` list in `internal/platform/arch/layering/blocks.go` in the same commit, or both layering guards fail on a package in no block.

`web/static/openapi.yaml` and `docs/API.md` are part of the published contract and carry no ratchet, so they are updated in the same change rather than after it.

## Open Questions

- **Does the fetch need the crawl proxy?** The ingest fleet routes through rotating exit IPs because several sources block datacenter ranges. LinkedIn answered the spike from a residential connection; whether it answers the production host the same way is unmeasured. Resolve by trying the plain path first from the production host and adding proxy egress only if it is actually refused — `safehttp` already has a proxy-aware constructor, so this is a configuration decision, not a redesign.
- **What rate limit?** The spec requires per-account throttling but does not fix a number. A small allowance per hour is enough for the intended use (a user imports their own profile once, maybe retries a typo) and leaves no room for enumeration.
