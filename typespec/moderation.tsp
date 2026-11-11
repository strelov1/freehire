import "@typespec/http";

using Http;

namespace Freehire;

model ModeratedUser {
  id: int64;
  email: string;
  username: string | null;
  role: string;
  banned: boolean;
  banned_at: utcDateTime | null;
  ban_reason: string | null;
}

model BanRequest {
  /** 1 to 500 characters. */
  reason?: string;
}

/** What a person reported about a company's hiring process. */
model ProcessReport {
  kind: string;

  /** How many people have reported this kind for the company. */
  count: int64;
}

model ProcessReportRequest {
  kind?: string;
}

// Moderators only (session cookie and the moderator role): look up an account by id,
// email or username, and ban or unban it. An admin cannot be banned, nor can the caller.

@tag("Moderation")
@route("/moderation/users")
@get
op findModeratedUser(@query q?: string): Data<ModeratedUser> | Problem;

@tag("Moderation")
@route("/moderation/users/{id}/ban")
@post
op banUser(@path id: string, @body body: BanRequest): Data<ModeratedUser> | Problem;

@tag("Moderation")
@route("/moderation/users/{id}/unban")
@post
op unbanUser(@path id: string): Data<ModeratedUser> | Problem;

// A signed-in person's reports about a company's hiring process (session cookie).

/** The kinds the caller has reported for the company. */
@tag("Feedback")
@route("/companies/{slug}/process-reports/mine")
@get
op myProcessReports(@path slug: string): Data<string[]> | Problem;

/** 409 when the caller already reported this kind. */
@tag("Feedback")
@route("/companies/{slug}/process-reports")
@post
op fileProcessReport(@path slug: string, @body body: ProcessReportRequest): {
  @statusCode _: 201;
  @body body: Data<ProcessReport>;
} | Problem;

@tag("Feedback")
@route("/companies/{slug}/process-reports")
@delete
op retractProcessReport(@path slug: string, @query kind?: string): Data<ProcessReport> | Problem;
