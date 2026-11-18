import type { Http } from '../api-core';
import type { EmailPrefs, EmailPrefsUpdate } from '../api-types';
import { jsonBody } from '../api-core';
import type { components } from '#lib/generated/api.js';

export function emailPrefsApi({ requestData }: Http) {
  /** The email preferences a signed unsubscribe link opens. Unauthenticated: the
   *  token in the link stands in for a session, which is the whole point — somebody
   *  holding one of our mails turns it off without an account. */
  function getEmailPrefs(token: string): Promise<EmailPrefs> {
    return requestData<EmailPrefs>(`/api/v1/email-prefs?t=${encodeURIComponent(token)}`);
  }

  /** Save the switches. The token goes in the BODY, not the query: nginx logs query
   *  strings, and this call has a body already, so there is no reason to write a
   *  never-expiring credential into the access log twice.
   *
   *  Every field except the token is optional, and an omitted switch keeps its
   *  stored value — this is a patch, not a replace. `deactivateSearches` can only
   *  turn a digest off; there is deliberately no way to turn one on from a link. */
  function saveEmailPrefs(token: string, update: EmailPrefsUpdate): Promise<EmailPrefs> {
    return requestData<EmailPrefs>(
      '/api/v1/email-prefs',
      jsonBody('PATCH', {
        token,
        alerts_enabled: update.alerts,
        activity_enabled: update.activity,
        news_enabled: update.news,
        deactivate_searches: update.deactivateSearches,
      } satisfies components['schemas']['EmailPrefsTokenUpdate']),
    );
  }

  /** The same three email-group switches for a signed-in caller. Shares the
   *  server-side service with the token-opened page, so the two views cannot drift
   *  apart about the same three booleans. */
  function getMyEmailGroups(): Promise<EmailPrefs> {
    return requestData<EmailPrefs>('/api/v1/me/email-groups');
  }

  /** Save them for a signed-in caller. A patch, like the public one: an omitted
   *  switch keeps its stored value. */
  function saveMyEmailGroups(update: EmailPrefsUpdate): Promise<EmailPrefs> {
    return requestData<EmailPrefs>(
      '/api/v1/me/email-groups',
      jsonBody('PATCH', {
        alerts_enabled: update.alerts,
        activity_enabled: update.activity,
        news_enabled: update.news,
        deactivate_searches: update.deactivateSearches,
      } satisfies components['schemas']['EmailPrefsUpdate']),
    );
  }

  return {
    getEmailPrefs,
    saveEmailPrefs,
    getMyEmailGroups,
    saveMyEmailGroups,
  };
}
