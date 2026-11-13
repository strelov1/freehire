import "@typespec/http";
import "./common.tsp";

using Http;

namespace Freehire;

// The in-app assistant's JSON routes and auto-apply management. The routes that stream
// (messages, retry, opening, sync, autopilot), the attachment upload and the queue-scoped
// tailoring run stay hand-written.

/** One conversation. */
model AssistantSession {
  id: string;
  preset: string;
  label: string;
  search_context?: unknown;
  cv_id?: string;
  job_id?: int64;
}

/** Optional body of a session create: binds a general chat to a fixed search. */
model AssistantSessionRequest {
  search_query?: string | null;
  request_id?: string;
}

/** A conversation with its full transcript; `messages` is the stored transcript as is. */
model AssistantSessionDetail {
  session: AssistantSession;
  messages: unknown;
}

model AssistantSessionListMeta {
  total: int64;
}

model AssistantSessionList {
  data: AssistantSession[];
  meta: AssistantSessionListMeta;
}

/** The 402 a spent or missing allowance answers with. */
model AssistantPlanRefusal {
  error: string;
  allowance: unknown;

  /** Absent for a fair-use refusal and for a caller already on the top plan. */
  upgrade_url?: string;
}

model AssistantExtension {
  turns: int64;
  ceiling: int64;
  unlimited: boolean;
  allowance: unknown;
}

model AssistantVoiceToken {
  value: string;
  `model`: string;
  calls_url: string;
}

model AssistantVoiceTurn {
  /** `user` or `assistant`. (e.g. `user`) */
  role?: string;
  /** The transcribed/spoken text. */
  content?: string;
}

// Starts a conversation. `preset` picks the kind; `job` names the application an
// interview, debrief or apply session is held against. A session already held for the
// same application, or the same search request, is handed back (200); a new one is a
// 201.
/**
 * For shortlisting the current jobs list, pass search_query (the complete serialized
 * query; an empty string is valid) and a client-generated request_id UUID. Only chat
 * sessions accept this binding. Reusing the UUID with the same owner and canonical query
 * returns the same session; changing its query returns 409. Unknown search parameters
 * return 400 at creation. A condition that cannot be enforced when a search runs (for
 * example hide_seen without a readable history) fails that search instead, and the
 * assistant reports it rather than widening the search. Search-bound responses include
 * search_context: { query, brief }; the original query is immutable and the selection
 * brief is local to the conversation.
 */
@summary("Start a new conversation.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    _: 200,
    body: #{
      data: #{
        id: "b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f",
        preset: "chat",
        label: "",
      },
    },
  },
})
@tag("Assistant")
@route("/assistant/sessions")
@post
op createAssistantSession(
  /**
   * One of `chat` (default), `profile`, `browse`, `interview`, `debrief`, `apply`. (e.g.
   * `chat`)
   */
  @query preset?: string,
  /**
   * Public slug of an application you hold. Required for `interview`/`debrief`/`apply`,
   * ignored otherwise. (e.g. `senior-go-engineer-acme-1a2b`)
   */
  @query job?: string,
  @body body?: AssistantSessionRequest,
):
  | {
      @statusCode _: 200;
      @body body: Data<AssistantSession>;
    }
  | {
      @statusCode _: 201;
      @body body: Data<AssistantSession>;
    }
  | Problem;

// The caller's chat conversations, newest activity first.
/**
 * Tailoring conversations are not chats — each belongs to a CV — so they never appear
 * here.
 */
@summary("List your chat conversations, newest activity first.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: "b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f",
        preset: "chat",
        label: "Relocating to Berlin?",
      },
    ],
    meta: #{
      total: 1,
    },
  },
})
@tag("Assistant")
@route("/assistant/sessions")
@get
op listAssistantSessions(): AssistantSessionList | Problem;

/** One owned conversation with its transcript. */
@summary("One owned conversation with its full transcript.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      session: #{
        id: "b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f",
        preset: "chat",
        label: "Relocating to Berlin?",
      },
      messages: #[
        #{
          seq: 1,
          role: "user",
          content: #{
            text: "Should I relocate for this role?",
          },
        },
        #{
          seq: 2,
          role: "assistant",
          content: #{
            text: "...",
            `...`: "...",
          },
        },
      ],
    },
  },
})
@tag("Assistant")
@route("/assistant/sessions/{id}")
@get
op getAssistantSession(/** The session id. (e.g. `b2f1c2b0-6e2a-4c9e-9c2e-0a1b2c3d4e5f`) */ @path id: string): Data<AssistantSessionDetail> | Problem;

// Removes an owned conversation and its transcript.
/** Returns `204 No Content`. */
@summary("Delete an owned conversation and its transcript.")
@useAuth(SessionOrKey)
@tag("Assistant")
@route("/assistant/sessions/{id}")
@delete
op deleteAssistantSession(/** The session id. */ @path id: string): {
  @statusCode _: 204;
} | Problem;

// Stops the turn running in the session; a session with nothing running is not an
// error.
/**
 * A no-op (still `204`) when nothing is running — you cannot see whether a turn is live
 * before asking.
 */
@summary("Stop the session’s running turn.")
@useAuth(SessionOrKey)
@tag("Assistant")
@route("/assistant/sessions/{id}/cancel")
@post
op cancelAssistantTurn(/** The session id. */ @path id: string): {
  @statusCode _: 204;
} | Problem;

// Buys a tailoring session another ceiling's worth of turns. 204 when plan limits are
// not wired.
/**
 * A CV editing session is bounded by a turn ceiling as well as by the daily session
 * allowance, and a turn past the ceiling is a `402` naming the session. This spends
 * another of the day’s CV editing sessions to raise it (no body), and is idempotent under
 * a double click — two calls in flight buy one ceiling, not two. `409` on any preset
 * other than a CV editing session: a chat is bounded by the daily assistant allowance,
 * and a day cannot be topped up. `402` when there is no session left to spend, with the
 * same body every refusal carries.
 */
@summary("Buy a CV editing session another ceiling’s worth of turns.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    _: 200,
    body: #{
      data: #{
        turns: 15,
        ceiling: 30,
        unlimited: false,
        allowance: #{
          feature: "tailor",
          used: 2,
          limit: 2,
          unlimited: false,
          enforced: false,
          resets_at: utcDateTime.fromISO("2026-09-01T00:00:00Z"),
        },
      },
    },
  },
})
@tag("Assistant")
@route("/assistant/sessions/{id}/extend")
@post
op postAssistantExtend(/** The session id. */ @path id: string):
  | {
      @statusCode _: 200;
      @body body: Data<AssistantExtension>;
    }
  | {
      @statusCode _: 204;
    }
  | {
      @statusCode _: 402;
      @body body: AssistantPlanRefusal;
    }
  | Problem;

// Mints the short-lived credential for a voice call on an interview session.
/**
 * Only on an `interview` session; `409` otherwise. The audio itself never reaches this
 * server: take the returned `value` to `calls_url` (this deployment’s own realtime
 * gateway, not api.openai.com directly) to negotiate the WebRTC call. `501` when voice
 * mode is not configured.
 */
@summary("Mint a short-lived credential for a hands-free voice call.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      value: "ek_...",
      `model`: "gpt-realtime",
      calls_url: "https://freehire.me/api/v1/realtime/calls",
    },
  },
})
@tag("Assistant")
@route("/assistant/sessions/{id}/voice-token")
@post
op postAssistantVoiceToken(/** The session id. */ @path id: string): Data<AssistantVoiceToken> | Problem;

// Appends one completed spoken turn to an interview session's transcript.
/**
 * Only on an `interview` session. Lets a call continued by typing carry the spoken turns
 * forward in context. Returns `204 No Content`.
 */
@summary("Append one completed spoken exchange to the transcript.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    id: "<id>",
    body: #{
      role: "user",
      content: "I led the migration to Kubernetes.",
    },
  },
})
@tag("Assistant")
@route("/assistant/sessions/{id}/voice-turns")
@post
op postAssistantVoiceTurn(/** The session id. */ @path id: string, @body body: AssistantVoiceTurn): {
  @statusCode _: 204;
} | Problem;

model AutoApplyQueued {
  status: string;
}

model AutoApplyRetried {
  retried: boolean;
}

model AutoApplyCancelled {
  cancelled: boolean;
}

model AutoApplyReviewRequest {
  /** `approved` or `declined`. */
  decision?: string;
}

model AutoApplyDecision {
  decision: string;
}

/** Starts an auto-apply attempt for the job; a repeat for a live attempt succeeds too. */
@tag("Auto-apply")
@route("/jobs/{slug}/auto-apply")
@post
op postJobAutoApply(@path slug: string):
  | Data<AutoApplyQueued>
  | {
      @statusCode _: 402;
      @body body: AssistantPlanRefusal;
    }
  | Problem;

/** Re-arms a stuck attempt, or refreshes the answer preview of one awaiting review. */
@tag("Auto-apply")
@route("/jobs/{slug}/auto-apply/retry")
@post
op postJobAutoApplyRetry(@path slug: string): Data<AutoApplyRetried> | Problem;

/** Withdraws the caller's attempt on the job. */
@tag("Auto-apply")
@route("/jobs/{slug}/auto-apply")
@delete
op deleteJobAutoApply(@path slug: string): Data<AutoApplyCancelled> | Problem;

/** Records the approve or decline decision for a tailored queue entry. */
@useAuth(SessionOrKey)
@tag("Auto-apply")
@route("/me/auto-apply/{queueId}/review")
@post
op postAutoApplyReview(@path queueId: string, @body body: AutoApplyReviewRequest):
  Data<AutoApplyDecision> | Problem;
