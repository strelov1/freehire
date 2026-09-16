import { defineMessages } from '$lib/i18n/t';

// The chrome around a ledger event: what dates it, and where it leads.
//
// NOT the event's own caption. What KIND of event it was is `eventLabel` in `$lib/events`,
// which is English for all three of its consumers (this list, the calendar's cells and the
// job drawer). Translating that vocabulary is one change to `$lib/events` and its three
// callers, and doing half of it here — Russian chrome around an English caption, in the one
// component that happens to have a catalog — would be worse than the gap it patches.
export const messages = defineMessages(
  {
    /** Shown instead of a clock for an event nobody but the candidate dated. `appevent` owns
     *  that verdict server-side; this only renders it. */
    recordedByYou: 'recorded by you',
    applicationLink: 'application',
    messageLink: 'message',
  },
  {
    ru: {
      recordedByYou: 'записано вами',
      applicationLink: 'отклик',
      messageLink: 'письмо',
    },
  },
);
