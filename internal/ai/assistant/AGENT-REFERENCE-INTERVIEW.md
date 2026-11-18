# The in-app assistant: interview rehearsal, voice and debrief

Part of the [assistant reference](AGENT-REFERENCE.md) (its "How it works"): the `interview` preset, its voice mode, and the `debrief` that mirrors it.

**Interview rehearsal** (`handler/assistant_interview_tools.go`) is a mock interview against
one application. It is minted from `POST /assistant/sessions?preset=interview&job=<slug>`,
binds to a vacancy and to NO CV, and opens itself: `POST /assistant/sessions/:id/opening`
runs one turn under a server-side brief, the way autopilot does, because the candidate
arrived from an application with nothing to type. That endpoint refuses only an ANSWERED
opening — a transcript holding an assistant message. The runner records the brief before
it calls the model, so a turn that dies upstream leaves one user line behind, and
refusing on that would strand the session with no way to retry. A reload of a live
conversation replays it rather than restarting the interview.

Three decisions carry it:

- **The application row is the authorisation.** `user_jobs` holds one row per (user,
  vacancy), so its absence answers "not yours" and "no such thing" with the same 404. The
  same read supplies the stage.
- **The invitation is placed by the server, not fetched by the agent.** `interview_context`
  carries the employer's most recent `interview_invitation` for the application, flagged
  untrusted in the payload itself. Registering the seven inbox tools to retrieve one
  predictable fact would be paid for on every turn — and `inbox.Service.InterviewInvitation`
  keeps the read inside the package whose rule it is, so `read_at` stays untouched.
- **Search finds by meaning; only `experience_get` finds by id.** `experience_search` scores
  a query against the owner's atoms and drops zero-scoring matches, so an id passed as a
  query matches nothing — and an empty search result means "no such evidence". Three
  surfaces hand the agent ids (an opening message naming a selection, `get_profile`'s
  `soft_duplicate_clusters`, the id a merge returns), and before the read tool existed the
  interviewer reported the candidate's own achievements as non-existent and answered about
  a different set. `experience_get` resolves ids in one owner-scoped pass, reports
  unresolvable ones instead of failing, and caps a call while naming what it did not read.
  A foreign id reads exactly like a deleted one.
- **The bank gate is enforced in code, not trusted to the prompt.** `experience_add`
  takes `said` as a quote, and `provenanceFor` checks it verbatim against what the
  candidate actually typed in this session (`assistant.UserSaid` over the transcript,
  whitespace-collapsed and case-insensitive, otherwise literal). A quote that appears
  is stamped `stated_in_chat`; one that does not — a paraphrase, a summary, an
  invention — is stamped `agent_inferred` and barred from CVs. An unverifiable
  transcript fails closed the same way. A rehearsal is where people improvise, and an
  improvisation banked as evidence is a claim they never made.

The preset carries the discovery, tracking and bank tools plus `interview_context`, and
NOT the CV tools, the mail tools or `read_current_page`. It runs on the ordinary step
ceiling: a rehearsal is a dialogue, not an unattended pass.

**Voice mode** (`handler/assistant_interview_voice.go`) is a hands-free spoken call
on an `interview` session, over OpenAI's Realtime API via WebRTC — the browser
connects directly to OpenAI, so audio never transits this process. It exists beside
the ordinary tool-calling turn loop, not inside it:

- `POST /assistant/sessions/:id/voice-token` mints a short-lived Realtime client
  secret. Its `instructions` carry a ONE-SHOT rendering of `rehearsalContext`
  (`voiceInterviewInstructions`) rather than a live `interview_context` tool call —
  a voice session has no data-channel tool relay, so what the text preset fetches on
  demand is baked in once, at mint time, and does not change mid-call.
- **No tool calls in a voice turn.** `experience_add` and every other rehearsal tool
  are text-only. Nothing said on a call can be written to the candidate's experience
  bank, regardless of what they confirm out loud — extending this needs a real tool
  relay (client-side JS answering a Realtime function-call event against our REST
  API), not a corollary of shipping the call itself.
- `POST /assistant/sessions/:id/voice-turns` appends one completed exchange through
  the same `Store.Append` `Runner.persist` uses, so the transcript reads the same
  whether a turn was typed or spoken, and a candidate can drop into text mid-call
  without losing what was said out loud. It is NOT how `interview-debrief` learns
  what happened — that preset never reads another session's transcript (see below) —
  persistence is for this session's own history view and for mid-session continuity.
- The credential rides the same `LLM_BASE_URL`/`LLM_API_KEY` gateway as everything
  else here, gated by its own `REALTIME_MODEL` (no default, same posture as
  `STTModel` in `internal/platform/config` — unset means the feature is absent, not billed for
  by accident).
- **`REALTIME_MODEL` must carry the provider prefix** (`openai/gpt-realtime-2.1`, not
  the bare `gpt-realtime-2.1`), unlike every other model name in this file. It has
  outlived the gateway it was discovered on, which is the reason to keep both halves
  written down. On the litellm proxy the realtime WebRTC feature was lazy-loaded and did
  not resolve a `model_name` alias the way the rest of the proxy did —
  `can_key_call_resolved_model` accepted the alias and the upstream call then failed with
  `litellm.BadRequestError: LLM Provider NOT provided` (confirmed 2026-08-10). Production
  moved to Bifrost on 2026-08-31, where the same prefixed string answers 200 and mints an
  ephemeral key, and where nothing named `realtime` is a tier alias at all — audio is
  entitled by model name. Either way the client passes the provider-prefixed string.
- **Bifrost carries the call's media, not just the credential.** It terminates the
  browser's WebRTC connection and relays audio to OpenAI, so the gateway host must be
  reachable over UDP. Upstream advertises its Docker container's private address and
  no browser ever connects — a 200 from `/realtime/calls` followed by ICE stuck in
  `checking`. Production runs a patched build on one public UDP port
  (`provision/bifrost/WEBRTC-PATCH.md` in freehire-ops); going back to the upstream
  image silently breaks voice mode again.

**Interview debrief** is the rehearsal's mirror: the review of an interview that has
already happened, minted from `POST /assistant/sessions?preset=debrief&job=<slug>`. It
shares everything with the rehearsal but the prompt — the same binding, the same
`interview_context`, the same tool set, the same self-opening endpoint. Both are covered
by one notion in the handler (`bindsToApplication`), and `openingBriefFor` is where the
set of presets that speak first is written down; a preset that returns no brief is one
`PostAssistantOpening` refuses.

It is a preset of its own rather than a mode inside `interview` because of one rule that
**inverts** between them. The rehearsal must police itself against banking what the
candidate invented on the spot — improvising is what a rehearsal is for. In a debrief the
candidate is recalling what they already said to an employer, banking it is the purpose
of the session, and the instruction becomes "record what they confirm, never a number you
supplied". One prompt holding both would leak each mode's instinct into the other.

`TestTheDebriefCarriesTheRehearsalsTools` compares the two registries for equality. Two
presets sharing a tool set is new here, and it is the prompt — not the tools — that is
allowed to differ; a tool added to one and forgotten in the other would be a debrief that
cannot read the interview it is reviewing.

The endpoint accepts a debrief at any stage. Somebody who sat an interview and never moved
their application's stage is exactly who it is for. (The tracker's stage-gated entry point,
`offersDebrief`, went with the tracker layout rework, so the SPA no longer offers it anywhere.)
