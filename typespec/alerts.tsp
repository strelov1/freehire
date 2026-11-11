import "@typespec/http";

using Http;

namespace Freehire;

// A signed-in person's saved searches and the alert subscriptions on them (session cookie
// only). Everything is owner-scoped: an id that is not the caller's is a 404. A numeric id
// travels as a string so a non-number keeps its 400 "invalid id".

/** A named filter snapshot. `query` is the canonical search query string the SPA replays. */
model SavedSearch {
  id: int64;
  name: string;
  query: string;
  derived_from_profile: boolean;
  created_at: utcDateTime | null;
  updated_at: utcDateTime | null;
}

/** An empty name is refused with a 400, like an out-of-range one. */
model SavedSearchCreate {
  name?: string;

  /** Empty is the valid "show all" snapshot. */
  query?: string;

  derived_from_profile?: boolean;
}

/** A partial update: an absent or null field is kept; an empty query is a real "show all". */
model SavedSearchUpdate {
  name?: string | null;
  query?: string | null;
}

model AlertListMeta {
  total: int64;
}

/** The saved searches, most recently updated first. */
model SavedSearchList {
  data: SavedSearch[];
  meta: AlertListMeta;
}

/** One alert on a saved search. `saved_search_name` is sent on the list only. */
model Subscription {
  id: int64;
  saved_search_id: int64;
  saved_search_name?: string;
  channel: string;
  active: boolean;
  created_at: utcDateTime | null;
}

model SubscriptionCreate {
  saved_search_id?: int64;

  /** Defaults to telegram when empty. */
  channel?: string;
}

model SubscriptionActive {
  active?: boolean;
}

model SubscriptionList {
  data: Subscription[];
  meta: AlertListMeta;
}

@tag("Alerts")
@route("/me/searches")
@get
op listSavedSearches(): SavedSearchList | Problem;

/** 400 for a bad name, 409 for a duplicate name or the per-user cap. */
@tag("Alerts")
@route("/me/searches")
@post
op createSavedSearch(@body body: SavedSearchCreate): {
  @statusCode _: 201;
  @body body: Data<SavedSearch>;
} | Problem;

@tag("Alerts")
@route("/me/searches/{id}")
@patch
op updateSavedSearch(@path id: string, @body body: SavedSearchUpdate): Data<SavedSearch> | Problem;

@tag("Alerts")
@route("/me/searches/{id}")
@delete
op deleteSavedSearch(@path id: string): NoContentResponse | Problem;

@tag("Alerts")
@route("/me/subscriptions")
@get
op listSubscriptions(): SubscriptionList | Problem;

/** A non-owned saved search is a 404, a duplicate a 409. */
@tag("Alerts")
@route("/me/subscriptions")
@post
op createSubscription(@body body: SubscriptionCreate): {
  @statusCode _: 201;
  @body body: Data<Subscription>;
} | Problem;

/** Pauses or resumes. */
@tag("Alerts")
@route("/me/subscriptions/{id}")
@patch
op setSubscriptionActive(@path id: string, @body body: SubscriptionActive): Data<Subscription> | Problem;

@tag("Alerts")
@route("/me/subscriptions/{id}")
@delete
op deleteSubscription(@path id: string): NoContentResponse | Problem;
