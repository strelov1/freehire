# freehire architecture: three request flows

Part of [freehire architecture](../architecture.md): how a request travels when someone finds a job, tailors a CV, and asks the assistant.

## Flow: finding a job

The search path is the most-travelled code in the system, and its defining property is that a result page is served **without touching Postgres for the payload**. The Meilisearch document *is* the public wire shape of a job, so the index answers the whole query.

<img src="diagrams/flow-finding-a-job.svg" alt="A search request served without touching Postgres for the payload: the reader's search request is translated to a Meilisearch filter and answered from the index; Postgres is queried only afterward, best-effort, for the ghost-job signal on that page.">

A few things are load-bearing here.

**The index stores the wire shape.** `search.JobDocument` embeds `jobview.Job`, so a hit renders with the same SPA components as a detail page and needs no hydration. The one difference is the description, which is capped at 1000 runes in the index to keep a rebuild small enough to swap within the host's free disk; the detail endpoint serves the full text. The separate `GET /api/v1/agent/jobs/search` endpoint runs the identical query and *does* rehydrate full descriptions from Postgres, for programmatic consumers that want to screen a set in one call.

**Postgres is still touched, but only for the ghost signal.** The absence stamps behind that signal cannot live in the index — nothing in the crawl writes them, so they would never reach Meilisearch on their own. The lookup is best-effort by design: a failure leaves the badge off the page rather than failing the search.

**Sorting has an opinion.** With no explicit sort, an empty query defaults to freshest-first — relevance is meaningless without text — while a text query keeps relevance order.

**Deep pagination is refused, not slowed.** The window guard caps `offset + limit` at 10,000, which is roughly 500 pages and far beyond real browsing; the reported total is allowed to count higher than the window allows you to reach.

One operational trap is worth knowing because it has caused outages: adding a new *filterable* attribute opens a window where the app emits a filter the live index does not yet accept, and Meilisearch answers a 400 that the handler maps to a 500 — so the whole filtered page breaks rather than degrading. Push the index settings first, or reindex before rolling out the image.

Deeper: [`internal/search`](../../internal/search/AGENTS.md) · [`internal/job/jobview`](../../internal/job/jobview/AGENTS.md) · [`internal/job/ghost`](../../internal/job/ghost/AGENTS.md)

## Flow: tailoring a CV

The CV workspace is built on a rule that shapes every part of it: **the agent may not write a claim the candidate has not made.** Enforcement is not a prompt instruction — it is an evidence gate in the write path, and the evidence comes from a durable store called the experience bank.

<img src="diagrams/flow-tailoring-a-cv.svg" alt="A tailoring turn cites the experience bank for every edit it writes: the candidate's CV is imported additively into the experience bank; a tailoring turn loops per vacancy requirement, searching the bank for scored evidence and writing each CV edit through the evidence-gated editor, then renders the result with Typst.">

*The upload-time `resumeextract` call is drawn as a self-message on `API` rather than its own lifeline.*

**The bank is a store, not a cache.** Everything else about a candidate's experience is derived and replaced — the structured extract is regenerated on every upload, a tailored CV is discarded with its vacancy — but the bank accumulates, and only its owner removes anything. That is why import is additive: someone who uploads a trimmed one-page CV must not lose the history they built.

**Provenance decides publication.** Every banked achievement records whether the *candidate* asserted it or the *model* inferred it. Only the former may be written into a CV, an unknown provenance fails closed, and the check lives in the service path rather than in a system prompt. In the interview-rehearsal preset this goes further still: a quote the agent attributes to the candidate is checked verbatim against what they actually typed in that session, and a paraphrase is stamped as the model's inference and barred.

**There is exactly one writer.** Nothing outside `internal/candidate/cvedit` writes a stored CV, and the seam is held by absence — the CV package declares no method that could. Every entry point goes through it: the editor's autosave, the template picker, the API's `PATCH`, the assistant's `cv_edit`, seeding a tailored copy. A whole-document `PUT` is an *input format*, not a second write path; a differ derives the operations, so from the editor's point of view it is indistinguishable from an agent's batch. Each edit records both what it did and what would undo it, written with the document in one transaction against a locked row — which is also what stops two agent turns interleaving on one CV.

**Rendering is sandboxed.** Templates are Typst source files embedded in the binary. The renderer shells out to the Typst CLI in a temporary root with system fonts disabled and bundled fonts staged in; candidate data reaches it through a JSON file, never through command arguments. The font set is deliberately small and metric-compatible with what résumé parsers expect.

Deeper: [`internal/candidate/experience`](../../internal/candidate/experience/AGENTS.md) · [`internal/candidate/cvedit`](../../internal/candidate/cvedit/AGENTS.md) · [`internal/candidate/cv`](../../internal/candidate/cv/AGENTS.md) · [`internal/candidate/resume`](../../internal/candidate/resume/AGENTS.md)

## Flow: asking the assistant

The assistant runs **in this process**. There is no external agent runtime, no shell, and no credential minted for it: a tool receives the session owner's user id and calls the same Go service the HTTP handler calls. Anything the agent must not reach is simply a tool that does not exist.

<img src="diagrams/flow-asking-the-assistant.svg" alt="A bounded tool-calling loop that always ends in exactly one terminal event: the Runner calls the LLM gateway on the caller's own credential each round, then either calls a tool and streams the result back or streams a final answer; every path ends in exactly one terminal SSE event.">

*The `≤ MaxSteps` iteration is drawn as an annotation rather than a wrapping `loop` frame, to keep the diagram to one combined fragment (the `alt` on tool-calls vs. answer text).*

**A turn is bounded twice** — by tool-calling rounds and by the model client's per-call timeout — and both bounds are chosen server-side, because a ceiling a client can raise is not a ceiling. Zero or negative values fall back to defaults rather than meaning "unbounded"; an unbounded loop on a metered gateway is a runaway bill.

**Every turn ends with exactly one terminal event**, on every path: an answer, the step cap, a cancellation, a failure. A client that receives none waits forever.

**A turn outlives its reader.** A failed SSE write means this reader is not listening — a phone freezing a backgrounded tab, a slept laptop — and nothing more. The turn runs to its own end and its transcript is stored whether or not anyone reads it. Treating that write as "the user left" once threw away an unattended tailoring run's report after twenty-five committed CV edits. Stopping is therefore a request of its own, because a dropped connection cannot be told apart from a deliberate Stop.

**The transcript is the model's history** — one table holds both, tool calls and tool results included, so there is nothing for two stores to drift about.

**Presets select a prompt and a tool set, and nothing else**, which is how one chat component serves the website, the CV-tailoring workspace, the experience interview, and the extension's side panel. The mail tools are instructive about the boundary: no tool opens a message by id, because that marks it read and an agent sweeping the backlog would zero its owner's unread count; and no tool sends mail, because message bodies are attacker-controlled text and the surest answer to a prompt injection is that it has no outbound channel.

Every turn is billed to the caller's own gateway credential, tagged by feature and preset. Attribution fails open — it can never refuse or fail a call.

Deeper: [`internal/ai/assistant`](../../internal/ai/assistant/AGENTS.md) · [`internal/api/handler`](../../internal/api/handler/AGENTS.md) · [`internal/platform/llm`](../../internal/platform/llm/AGENTS.md) · [`internal/ai/llmkey`](../../internal/ai/llmkey/AGENTS.md) · [`internal/ai/browsertools`](../../internal/ai/browsertools/AGENTS.md)
