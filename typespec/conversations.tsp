import "@typespec/http";

using Http;

namespace Freehire;

// The messaging core: every conversation of every kind, its messages, read state, blocks
// and reports. Every route is session-cookie only; the report queue is moderators only.

/** A buddy's card, reduced to what an invitation shows. */
model ConversationCardSummary {
  category: string;
  seniority: string;
  country: string;
}

/** The buddy persona's own shape. Nothing here names the account behind it. */
model BuddyCounterpart {
  handle: string;
  card_summary: ConversationCardSummary;
}

/** The other side, as the persona they speak as draws them. `detail` is null when not reachable. */
model ConversationCounterpart {
  persona: string;
  reachable: boolean;
  detail: BuddyCounterpart | null;
}

/** `unread` and `last_message_at` mean something on the list only; elsewhere they are 0 and null. */
model Conversation {
  id: int64;
  kind: string;
  status_for_me: string;
  counterpart: ConversationCounterpart;
  unread: int64;
  last_message_at: utcDateTime | null;
  created_at: utcDateTime;
}

model ConversationMessage {
  id: int64;
  mine: boolean;
  body: string;
  created_at: utcDateTime;
}

model ConversationReportMessage {
  sender: string;
  body: string;
  created_at: utcDateTime;
}

/** Each account is named the way the conversation's kind draws it, so a moderator reads what the participants saw. */
model ConversationReport {
  id: int64;
  conversation_id: int64;
  kind: string;
  reason: string;
  note: string;
  created_at: utcDateTime;
  resolved_at: utcDateTime | null;
  reporter: string;
  reported: string;
  messages: ConversationReportMessage[];
}

model ConversationReportsMeta {
  limit: int64;
  offset: int64;
}

model ConversationReportsPage {
  data: ConversationReport[];
  meta: ConversationReportsMeta;
}

model ConversationReportRequest {
  reason?: string;
  note?: string;
}

model ConversationSendRequest {
  body?: string;
}

model ConversationReadRequest {
  up_to?: int64;
}

/** The caller's conversations of every kind, newest message first. */
@tag("Conversations")
@route("/me/conversations")
@get
op listConversations(): Data<Conversation[]> | Problem;

/** Opens a pending conversation the caller did not start. The id is declared a string and parsed in Go, which keeps the 400 text. */
@tag("Conversations")
@route("/me/conversations/{id}/accept")
@post
op acceptConversation(@path id: string): Data<Conversation> | Problem;

/** Refuses a pending conversation. */
@tag("Conversations")
@route("/me/conversations/{id}/decline")
@post
op declineConversation(@path id: string): Data<Conversation> | Problem;

/** Closes every conversation with the other participant and stops either from starting another, of any kind. */
@tag("Conversations")
@route("/me/conversations/{id}/block")
@post
op blockConversation(@path id: string): NoContentResponse | Problem;

/** Files a complaint about a conversation the caller takes part in. */
@tag("Conversations")
@route("/me/conversations/{id}/report")
@post
op reportConversation(@path id: string, @body body: ConversationReportRequest): NoContentResponse | Problem;

/** The messages after the given id, oldest first. Params that do not read fall back to their defaults. */
@tag("Conversations")
@route("/me/conversations/{id}/messages")
@get
op listConversationMessages(
  @path id: string,
  @query after?: string,
  @query limit?: string,
): Data<ConversationMessage[]> | Problem;

/** Posts a message into an open conversation. */
@tag("Conversations")
@route("/me/conversations/{id}/messages")
@post
op sendConversationMessage(@path id: string, @body body: ConversationSendRequest): {
  @statusCode _: 201;
  @body body: Data<ConversationMessage>;
} | Problem;

/** Records that the caller has read up to a message. */
@tag("Conversations")
@route("/me/conversations/{id}/read")
@post
op markConversationRead(@path id: string, @body body: ConversationReadRequest): NoContentResponse | Problem;

/** Moderators only: the reported conversations. Page params that do not read fall back to their defaults. */
@tag("Conversations")
@route("/moderation/conversation-reports")
@get
op listConversationReports(
  @query limit?: string,
  @query offset?: string,
  @query include_resolved?: string,
): ConversationReportsPage | Problem;

/** Moderators only: marks a report handled. Idempotent; 404 for an unknown id. */
@tag("Conversations")
@route("/moderation/conversation-reports/{id}/resolve")
@post
op resolveConversationReport(@path id: string): NoContentResponse | Problem;
