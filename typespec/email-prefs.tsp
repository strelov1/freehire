import "@typespec/http";

using Http;

namespace Freehire;

/** One email digest subscription the switches govern. */
model EmailPrefsSearch {
  id: int64;
  name: string;
  active: boolean;
}

/** The preference page's state: the address mail goes to and the three switches. */
model EmailPrefs {
  email: string;
  alerts_enabled: boolean;
  activity_enabled: boolean;
  news_enabled: boolean;
  searches: EmailPrefsSearch[];
}

/** A patch: an absent switch keeps its stored value. Searches can only be turned off. */
model EmailPrefsUpdate {
  alerts_enabled?: boolean;
  activity_enabled?: boolean;
  news_enabled?: boolean;
  deactivate_searches?: int64[];
}

/**
 * The token-opened patch. The token rides in the body so it never reaches an access
 * log. Optional on the wire because a missing token must be refused exactly like a
 * forged one (404), not with a 400 that tells the two apart.
 */
model EmailPrefsTokenUpdate {
  ...EmailPrefsUpdate;
  token?: string;
}

model OneClickResult {
  unsubscribed_from: string;
  manage_url: string;
}

// The page an emailed link opens. Unauthenticated by design: the signed token stands
// in for a session, and every refusal is the same 404. The two /me routes are the
// same switches for a signed-in caller (session cookie).

@useAuth(NoAuth)
@tag("Email preferences")
@route("/email-prefs")
@get
op getEmailPrefs(@query t?: string): Data<EmailPrefs> | Problem;

@useAuth(NoAuth)
@tag("Email preferences")
@route("/email-prefs")
@patch
op updateEmailPrefs(@body body: EmailPrefsTokenUpdate): Data<EmailPrefs> | Problem;

/** RFC 8058 one-click unsubscribe: turns off only the group the token names. */
@useAuth(NoAuth)
@tag("Email preferences")
@route("/email-prefs/one-click")
@post
op oneClickUnsubscribe(@query t?: string): Data<OneClickResult> | Problem;

@tag("Email preferences")
@route("/me/email-groups")
@get
op getMyEmailGroups(): Data<EmailPrefs> | Problem;

@tag("Email preferences")
@route("/me/email-groups")
@patch
op updateMyEmailGroups(@body body: EmailPrefsUpdate): Data<EmailPrefs> | Problem;
