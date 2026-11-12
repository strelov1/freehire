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

### `add_cv` only for a first setup, atomically
If the account already has a CV the tool refuses (409) and names `/my/profile`. Reasons:
`Put` REPLACES the stored document and re-derives its structure, so an agent could overwrite a
PDF the person chose with chat text; replacing is a deliberate act the website keeps in the
person's own session (`PUT /me/resume` is cookie-only). It also bounds the free extraction to
one per account over this channel. The check is `resume.Store.PutIfAbsent`. Each upload
writes its bytes to an object of its own (`resumes/<id>-<unix nanos>-<random>`) OUTSIDE any
transaction — an object-store round trip must never hold a pooled connection — then, in a short
transaction under a per-user `pg_advisory_xact_lock` that `Put` and `Delete` take too, checks
presence and moves the pointer. The superseded object is deleted after commit; on a refusal
or failure the just-written one is, unless a fresh read shows the pointer names it (a lost
COMMIT reply). That cleanup is best-effort; account deletion removes every object under the
user's prefix (it lists `resumes/<id>-`, plus the legacy `resumes/<id>`, before and once more
after the row is erased). Every reader takes the key from the pointer (the referral CV view
used to derive it from the user id and now reads through the store).

### Only the person's own CV; provenance `cv_import`
`add_cv` accepts a CV the person supplied — pasted CV text, or the content of a CV file they
gave the agent — passed verbatim. It does not accept a history the agent writes up from the
conversation: that text would be an outside model's, banked as `cv_import`, and only a prompt
would keep it honest, where the rule is that the check lives in the service path. A person with
no CV is sent to `/my/profile` to upload or write one. With that boundary, what happens is a CV
import — the text becomes the stored CV and each atom traces to it by its upload stamp — which
is exactly what `cv_import` records. `stated_in_chat` (a transcript freehire checked) is
impossible for an external client, and `agent_inferred` would make the person's own CV unusable.
The signed-in server acts only for the person who approved the connection (the answer bank's
`AuthorCandidate` precedent), and the tool is announced as a write so clients ask first.

### Skills merge, as on the website
The import folds skills found in the work history into the profile's skills once the background
read finishes (`experience.Store.SetProfileSkills`), exactly as a website upload does. `add_cv`
does not special-case it; its description and result say so, and `update_profile` removes any.

### Garbage and injection
Length is capped at 30,000 characters and floored at 100 (shorter is not a CV and would
spend the one extraction on nothing); both bounds are also in the input schema. The extraction already treats CV text as untrusted (PII
masking, `Sanitize` on every atom); the text is data to it, as an uploaded PDF is.

### `get_setup_status` is its own tool
`get_profile` answers "who is this person", is served to API keys and the in-app assistant,
and is replayed into context every turn; setup status crosses the CV, alerts, plan and
permissions and changes as the person acts. A separate read keeps `get_profile`'s shape. It is
grant-only: the CV status it reads is cookie-only over REST (`GET /me/resume`), and the step it
most often names (`add_cv`) is grant-only.

Steps in order: `cv`, `preferences` (specializations and skills set), `alerts` (an active
one), then the optional `paid_plan` and `agents_may_apply` (auto-apply prerequisites). A CV
whose reading failed sends the person to `/my/profile` to retry. `next_step` is the first CORE
step not done; once the core is done it points at a search, and the optional steps stay visible
in `steps` without being pushed.

## Risks

- A third-party model could still submit text that is not the person's CV despite the
  description. The person sees the imported history on the website and can edit or delete it.
