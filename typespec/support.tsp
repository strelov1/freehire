import "@typespec/http";

using Http;

namespace Freehire;

// The support agent. Open to every signed-in account and cookie only. Posting a message
// is Server-Sent Events and stays hand-written.

model SupportSession {
  id: string;
  preset: string;
  label: string;
  search_context?: unknown;
  cv_id?: string;
  job_id?: int64;
}

/**
 * Starts a support conversation. The blank one the caller already has is handed back
 * (200) rather than adding another; a new one is a 201.
 */
@tag("Support")
@route("/support/sessions")
@post
op createSupportSession():
  | {
      @statusCode _: 200;
      @body body: Data<SupportSession>;
    }
  | {
      @statusCode _: 201;
      @body body: Data<SupportSession>;
    }
  | Problem;

/** Stops the turn running in a support session. */
@tag("Support")
@route("/support/sessions/{id}/cancel")
@post
op cancelSupportTurn(@path id: string): {
  @statusCode _: 204;
} | Problem;
