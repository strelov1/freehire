import "@typespec/http";

using Http;

namespace Freehire;

// The interview-buddy network and its mock interviews. Every route is cookie-only.

model BuddyCardSummary {
  category: string;
  seniority: string;
  country: string;
}

model BuddyCompany {
  slug: string;
  name: string;
}

model BuddyStatus {
  member: boolean;
  handle?: string;
  card_empty: boolean;
}

model BuddySimilar {
  handle: string;
  card_summary: BuddyCardSummary;
  score: int64;
  shared_companies: BuddyCompany[];
}

/** Count per kind (no key for a kind nobody chose) and the viewer's own choice, "" for none. */
model BuddyReactions {
  counts: Record<int64>;
  mine: string;
}

model BuddyFeedEntry {
  id: int64;
  handle: string;
  card_summary: BuddyCardSummary;
  company: BuddyCompany;
  job_title: string | null;
  job_slug: string | null;
  job_location: string | null;
  reactions: BuddyReactions;
  type: string;
  day: string;
}

model BuddyFeedMeta {
  next_cursor: string | null;
}

model BuddyFeedPage {
  data: BuddyFeedEntry[];
  meta: BuddyFeedMeta;
}

model BuddyProfile {
  handle: string;
  card_summary: BuddyCardSummary;
  card: TalentCard;
  card_empty: boolean;
}

model BuddyActivityDay {
  day: string;
  count: int64;
  applied: int64;
  interview: int64;
  offer: int64;
}

model BuddyActivity {
  days: BuddyActivityDay[];
}

model BuddyReactBody {
  kind?: string;
}

model BuddyInviteBody {
  handle?: string;
  body?: string;
}

/** The persona's own detail; null when the other side is not reachable. */
model BuddyCounterpartDetail {
  handle: string;
  card_summary: BuddyCardSummary;
}

model BuddyConversationCounterpart {
  persona: string;
  reachable: boolean;
  detail: BuddyCounterpartDetail | null;
}

model BuddyConversation {
  id: int64;
  kind: string;
  status_for_me: string;
  counterpart: BuddyConversationCounterpart;
  unread: int64;
  last_message_at: utcDateTime | null;
  created_at: utcDateTime;
}

model BuddySimilarMeta {
  limit: int64;
  offset: int64;
}

model BuddySimilarPage {
  data: BuddySimilar[];
  meta: BuddySimilarMeta;
}

@tag("Buddies")
@route("/me/buddies")
@get
op buddyStatus(): Data<BuddyStatus> | Problem;

@tag("Buddies")
@route("/me/buddies")
@post
op buddyJoin(): Data<BuddyStatus> | Problem;

@tag("Buddies")
@route("/me/buddies")
@delete
op buddyLeave(): NoContentResponse | Problem;

@tag("Buddies")
@route("/me/buddies/similar")
@get
op buddySimilar(@query limit?: string, @query offset?: string): BuddySimilarPage | Problem;

@tag("Buddies")
@route("/me/buddies/feed")
@get
op buddyFeed(
  @query handle?: string,
  @query mode?: string,
  @query cursor?: string,
  @query limit?: string,
): BuddyFeedPage | Problem;

@tag("Buddies")
@route("/me/buddies/feed/{id}/reaction")
@put
op buddyReact(@path id: string, @body body: BuddyReactBody): Data<BuddyReactions> | Problem;

@tag("Buddies")
@route("/me/buddies/feed/{id}/reaction")
@delete
op buddyUnreact(@path id: string): Data<BuddyReactions> | Problem;

@tag("Buddies")
@route("/me/buddies/profiles/{handle}")
@get
op buddyProfile(@path handle: string): Data<BuddyProfile> | Problem;

@tag("Buddies")
@route("/me/buddies/profiles/{handle}/activity")
@get
op buddyActivity(@path handle: string): Data<BuddyActivity> | Problem;

@tag("Buddies")
@route("/me/buddies/hidden-applications")
@get
op buddyHiddenApplications(): Data<int64[]> | Problem;

@tag("Buddies")
@route("/me/buddies/hidden-applications/{id}")
@put
op buddyHideApplication(@path id: string): NoContentResponse | Problem;

@tag("Buddies")
@route("/me/buddies/hidden-applications/{id}")
@delete
op buddyShowApplication(@path id: string): NoContentResponse | Problem;

@tag("Buddies")
@route("/me/buddies/invite")
@post
op buddyInvite(@body body: BuddyInviteBody): {
  @statusCode _: 201;
  @body body: Data<BuddyConversation>;
} | Problem;

// Mock interviews.

model BuddyMockSettings {
  timezone?: string;
  meeting_url?: string;
  topics?: string[] | null;
  paused?: boolean;
}

model BuddyMockSettingsOut {
  timezone: string;
  meeting_url: string;
  topics: string[] | null;
  paused: boolean;
}

model BuddyMockRule {
  /** 0 (Sunday) through 6; null for a dated override. */
  weekday?: int64 | null;

  /** YYYY-MM-DD; null for a weekly rule. */
  date?: string | null;

  /** HH:MM. */
  start?: string;

  /** HH:MM; 24:00 means until midnight. */
  end?: string;
}

model BuddyMockRuleOut {
  id: int64;
  weekday: int64 | null;
  date: string | null;
  start: string;
  end: string;
  closure: boolean;
}

model BuddyMockWeekly {
  rules?: BuddyMockRule[];
}

model BuddyMockReview {
  rating: int64;
  comment: string;
}

model BuddyMock {
  id: string;
  role: string;
  counterpart: string | null;
  starts_at: utcDateTime;
  ends_at: utcDateTime;
  topic: string;
  status: string;
  meeting_url: string | null;
  can_cancel: boolean;
  can_review: boolean;
  my_review: BuddyMockReview | null;
  their_review: BuddyMockReview | null;
}

model BuddyMockList {
  upcoming: BuddyMock[];
  past: BuddyMock[];
}

model BuddyMockSlot {
  starts_at: utcDateTime;
  ends_at: utcDateTime;
  local_start: string;
  local_end: string;
  utc_offset: string;
}

model BuddyMockSlotsMeta {
  timezone: string;
  count: int64;
  topics: string[] | null;
}

model BuddyMockSlotsPage {
  data: BuddyMockSlot[];
  meta: BuddyMockSlotsMeta;
}

model BuddyMockNextSlot {
  starts_at: utcDateTime;
  local_start: string;
  utc_offset: string;
}

model BuddyMockHost {
  handle: string;
  card_summary: BuddyCardSummary;
  topics: string[] | null;
  next_slot: BuddyMockNextSlot;
  rating_avg: float64;
  rating_count: int64;
  completed: int64;
}

model BuddyMockHostsMeta {
  timezone: string;
  count: int64;
}

model BuddyMockHostsPage {
  data: BuddyMockHost[];
  meta: BuddyMockHostsMeta;
}

model BuddyMockBook {
  starts_at?: utcDateTime;
  topic?: string;
  timezone?: string;
}

model BuddyMockReviewBody {
  rating?: int64;
  comment?: string;
}

model BuddyRevealedReview {
  rating: int64;
  comment: string;
  topic: string;
  day: string;
}

model BuddyMockSummary {
  completed: int64;
  rating_count: int64;
  rating_avg: float64;
  recent: BuddyRevealedReview[];
}

@tag("Buddies")
@route("/me/buddies/mocks")
@get
op buddyMockList(): Data<BuddyMockList> | Problem;

@tag("Buddies")
@route("/me/buddies/mocks/hosts")
@get
op buddyMockHosts(@query topic?: string, @query timezone?: string): BuddyMockHostsPage | Problem;

/** The caller's own settings, or null before the first save. */
@tag("Buddies")
@route("/me/buddies/mocks/settings")
@get
op buddyMockSettingsGet(): Data<BuddyMockSettingsOut | null> | Problem;

@tag("Buddies")
@route("/me/buddies/mocks/settings")
@put
op buddyMockSettingsSave(@body body: BuddyMockSettings): Data<BuddyMockSettingsOut> | Problem;

@tag("Buddies")
@route("/me/buddies/mocks/availability")
@get
op buddyMockAvailabilityGet(): Data<BuddyMockRuleOut[]> | Problem;

@tag("Buddies")
@route("/me/buddies/mocks/availability/weekly")
@put
op buddyMockWeeklyReplace(@body body: BuddyMockWeekly): NoContentResponse | Problem;

@tag("Buddies")
@route("/me/buddies/mocks/availability/overrides")
@post
op buddyMockOverrideAdd(@body body: BuddyMockRule): {
  @statusCode _: 201;
  @body body: Data<BuddyMockRuleOut>;
} | Problem;

@tag("Buddies")
@route("/me/buddies/mocks/availability/{id}")
@delete
op buddyMockRuleDelete(@path id: string): NoContentResponse | Problem;

@tag("Buddies")
@route("/me/buddies/mocks/{id}/cancel")
@post
op buddyMockCancel(@path id: string): NoContentResponse | Problem;

@tag("Buddies")
@route("/me/buddies/mocks/{id}/review")
@post
op buddyMockReview(@path id: string, @body body: BuddyMockReviewBody): NoContentResponse | Problem;

@tag("Buddies")
@route("/me/buddies/profiles/{handle}/mock-slots")
@get
op buddyMockSlots(
  @path handle: string,
  @query from?: string,
  @query to?: string,
  @query timezone?: string,
): BuddyMockSlotsPage | Problem;

@tag("Buddies")
@route("/me/buddies/profiles/{handle}/mocks")
@post
op buddyMockBook(@path handle: string, @body body: BuddyMockBook): {
  @statusCode _: 201;
  @body body: Data<BuddyMock>;
} | Problem;

@tag("Buddies")
@route("/me/buddies/profiles/{handle}/mock-reviews")
@get
op buddyMockReviews(@path handle: string): Data<BuddyMockSummary> | Problem;
