## Context

The assistant runs in-process (`internal/assistant`): a turn is a bounded tool-calling
loop, and a tool is a name, a JSON schema and a `func(ctx, userID, args)` — no shell,
no credential. Its registry holds market tools and tracking tools. It holds nothing
that reads the user's own saved preferences, which is why every conversation opens
with a questionnaire.

Two things the user has already told the product are relevant: the profile
(`userprofile.Profile` — specializations, skills, excluded skills, location
preferences) and the structured CV (`resumeextract.Structured`), which is where
seniority lives; `hardconstraint_inputs.go` already reads it for exactly that.

## Goals / Non-Goals

**Goals:**

- The agent starts from what the user has stated, and asks only about the rest.
- A user with no profile is sent to the page that persists their answers, rather than
  being interviewed for values that evaporate when the chat ends.
- Nothing personally identifying enters the conversation transcript by default.

**Non-Goals:**

- Writing the profile from chat. The profile page owns authoring.
- Reworking the CV-tailoring session, which has its own prompt and tools.

## Decisions

### The tool returns the same assembly the HTTP endpoint serves

`get_profile` is built on `profileHandlers`, not on the services beneath it, so the
tool and `GET /me/profile` cannot drift: one function assembles the profile plus the
contact-free CV, and both callers use it. That the endpoint also gained the `cv` block
is what makes this possible — and it earns its keep on its own, since the `freehire`
CLI needs the same grounding and reaches it over HTTP.

### The CV is projected by whitelist, and `internal/pii` is not involved

`Professional` names the fields it carries. A field added to `Structured` later is
withheld until someone adds it here too — the safe direction for a projection whose
job is to hold personal data back. A blacklist over the four known contact keys fails
the other way, which is precisely how `candidateContext` was built and why it is
moved onto the projection in this change.

`internal/pii` is a client to a separate service, for free text where the location of
personal data is unknown. Here the input is a typed struct whose contact fields are
known by name, so the hop buys nothing. It is also unnecessary: `resumeextract.Extract`
already severs contacts fail-closed — the model sees `[REDACTED_…]` placeholders and
the four fields are filled deterministically from detection spans afterwards, so the
free-text fields it produced could not have picked up a name.

### Why contacts matter more in a tool result than in an HTTP response

A tool result is persisted in the session transcript and replayed into the model's
context on every later turn. An HTTP response lives until the next request. So the
projection is not decoration on a surface that "also" has personal data — it is the
difference between a name appearing once and a name sitting in the conversation for
its whole life.

### A missing profile is a result, not an error

Returning an error would make the model treat it as a broken tool and fall back to
asking. Returning an empty profile would read as "this user has no preferences".
Returning `{profile: null, next: "…point them at /my/profile…"}` tells the model both
the fact and what to do with it, in the one place the model is guaranteed to read.

## Risks / Trade-offs

- **The prompt is the enforcement.** Nothing makes the model call `get_profile` first;
  the tool description and the chat prompt both say to. A prompt test pins that the
  instruction exists, which is as far as a unit test can go — whether the model obeys
  is observable only in a real conversation.
- **One more tool in every session's schema list.** Small, and the tool takes no
  arguments; the cost is a few hundred tokens of schema against a questionnaire that
  currently burns a whole turn.
- **The profile read now accepts an API key.** A leaked key can read a profile. It
  cannot write one — `PUT`/`DELETE` stay cookie-only — and the response carries no
  contacts, which stay on the cookie-only `/me/resume`.

## Migration Plan

No schema change, no migration, no config. The projection, the endpoint and the tool
ship together; the CLI can adopt `GET /me/profile`'s `cv` block whenever it likes.

## Open Questions

None.
