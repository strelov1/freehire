## Context

The website takes a CV through `PUT /me/resume` and `POST /me/resume/extract` (JSON `{text}`
is the paste path). Both store the bytes (`resume.Store.Put`) and start
`deriveResumeArtifacts`: a background, best-effort structured extraction that marks its own
context `plan.WithFreeAI` (the stored-CV parse is one of the three free model calls), then
imports the work history into the experience bank via `experience.Store.Import`, which stamps
every atom `cv_import`. The deterministic dictionaries (`resumeProfile`) give skills, seniority
and categories synchronously; onboarding turns them into suggested profile values.

## Decisions

### One upload use case, three callers
`resumeHandlers.storeCV(ctx, userID, up)` = `Put` + `deriveResumeArtifacts`. `PutResume`,
`ExtractResumeProfile` and `add_cv` call it. The free-AI mark stays where it is, inside the
background derivation, so the tool opens no new free model call: it reaches the same one the
website's paste does. No `mw.paidAI` is involved on either path.

### `add_cv` only for a first setup
If the account already has a CV the tool refuses (409) and names `/my/profile`. Reasons:
`Put` REPLACES the stored document and re-derives its structure, so an agent could overwrite a
PDF the person chose with chat text; replacing is a deliberate act the website keeps in the
person's own session (`PUT /me/resume` is cookie-only). It also bounds the free extraction to
one per account over this channel. The experience bank is additive, so nothing would be lost
there, but the document and its structure would be.

### Provenance: `cv_import`
The bank derives a label from the entry point, never from the request body. The text arrives
through one entry point whether the person pasted a CV or dictated their history, so the two
cannot honestly be told apart by a flag the agent sets. `stated_in_chat` means freehire checked
the words against a transcript it holds — impossible for an external client. `agent_inferred`
would make the person's own CV unusable for every CV feature. What happens is a CV import:
the text becomes the stored CV and each atom traces to it by its upload stamp, which is exactly
what `cv_import` records. The signed-in server acts only for the person who approved the
connection (the answer bank's `AuthorCandidate` precedent), the tool is announced as a write so
clients ask before calling it, and its description requires the person's own words, verbatim.

### Garbage and injection
Length is capped at 30,000 characters and floored at 100 (shorter is not a CV and would
spend the one extraction on nothing). The extraction already treats CV text as untrusted (PII
masking, `Sanitize` on every atom); the text is data to it, as an uploaded PDF is.

### `get_setup_status` is its own tool
`get_profile` answers "who is this person", is served to API keys and the in-app assistant,
and is replayed into context every turn; setup status crosses the CV, alerts, plan and
permissions and changes as the person acts. A separate read keeps `get_profile`'s shape. It is
grant-only: the CV status it reads is cookie-only over REST (`GET /me/resume`), and the step it
most often names (`add_cv`) is grant-only.

Steps in order: `cv`, `preferences` (specializations and skills set), `alerts` (an active
one), then the optional `paid_plan` and `agents_may_apply` (auto-apply prerequisites). A CV
whose reading failed sends the person to `/my/profile` to retry. `next_step` is the first step
not done; when all are done it points at a search.

## Risks

- A third-party model could submit embellished text as the person's CV. Mitigated as above;
  the person sees the imported history on the website and can edit or delete it.
