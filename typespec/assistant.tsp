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
  role?: string;
  content?: string;
}

/**
 * Starts a conversation. `preset` picks the kind; `job` names the application an
 * interview, debrief or apply session is held against. A session already held for the
 * same application, or the same search request, is handed back (200); a new one is a 201.
 */
@tag("Assistant")
@route("/assistant/sessions")
@post
op createAssistantSession(
  @query preset?: string,
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

/** The caller's chat conversations, newest activity first. */
@tag("Assistant")
@route("/assistant/sessions")
@get
op listAssistantSessions(): AssistantSessionList | Problem;

/** One owned conversation with its transcript. */
@tag("Assistant")
@route("/assistant/sessions/{id}")
@get
op getAssistantSession(@path id: string): Data<AssistantSessionDetail> | Problem;

/** Removes an owned conversation and its transcript. */
@tag("Assistant")
@route("/assistant/sessions/{id}")
@delete
op deleteAssistantSession(@path id: string): {
  @statusCode _: 204;
} | Problem;

/** Stops the turn running in the session; a session with nothing running is not an error. */
@tag("Assistant")
@route("/assistant/sessions/{id}/cancel")
@post
op cancelAssistantTurn(@path id: string): {
  @statusCode _: 204;
} | Problem;

/**
 * Buys a tailoring session another ceiling's worth of turns. 204 when plan limits are
 * not wired.
 */
@tag("Assistant")
@route("/assistant/sessions/{id}/extend")
@post
op postAssistantExtend(@path id: string):
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

/** Mints the short-lived credential for a voice call on an interview session. */
@tag("Assistant")
@route("/assistant/sessions/{id}/voice-token")
@post
op postAssistantVoiceToken(@path id: string): Data<AssistantVoiceToken> | Problem;

/** Appends one completed spoken turn to an interview session's transcript. */
@tag("Assistant")
@route("/assistant/sessions/{id}/voice-turns")
@post
op postAssistantVoiceTurn(@path id: string, @body body: AssistantVoiceTurn): {
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
@tag("Auto-apply")
@route("/me/auto-apply/{queueId}/review")
@post
op postAutoApplyReview(@path queueId: string, @body body: AutoApplyReviewRequest):
  Data<AutoApplyDecision> | Problem;
