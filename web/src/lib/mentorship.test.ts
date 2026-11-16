import { describe, expect, test } from 'vitest';
import { mentor, slot } from './mentorshipTestUtils';
import {
  dayStatuses,
  emptyMentorFilters,
  groupCalendarByLocalDay,
  profileInputFromProfile,
  seedFormFromSuggestions,
  openingSlotWindow,
  firstOpenMonth,
  withSearchParams,
  mentorFilterOptions,
  mentorFiltersFromParams,
  mentorFiltersToParams,
  mentorFiltersToQuery,
  seniorityLabel,
  canReview,
  companyLabel,
  formatInstantIn,
  isCancellable,
  splitAvailability,
  weekDraft,
  weekRows,
  nextWeekRange,
  exceptionDayLabel,
  weekdayLabel,
  weekdayOrder,
  weekdayShortLabel,
} from './mentorship';
import type {
  MentorAvailabilityRule,
  MentorCalendarInterval,
  MentorProfileInput,
  MentorProfileSuggestions,
  MentorSession,
  OwnMentorProfile,
} from './types';

function rule(over: Partial<MentorAvailabilityRule> = {}): MentorAvailabilityRule {
  return { id: 1, weekday: 1, date: null, start: '18:00', end: '21:00', closure: false, ...over };
}

function session(over: Partial<MentorSession> = {}): MentorSession {
  return {
    id: 'b1e2',
    mentor_slug: 'anna-k',
    headline: 'Staff Engineer',
    starts_at: '2026-10-15T16:00:00Z',
    ends_at: '2026-10-15T17:00:00Z',
    status: 'confirmed',
    note: '',
    meeting_url: 'https://meet.example.test/anna',
    completed: false,
    ...over,
  };
}

describe('mentor directory filters', () => {
  test('empty filters serialize to an empty query', () => {
    expect(mentorFiltersToParams(emptyMentorFilters()).toString()).toBe('');
  });

  test('the six filters the directory reads survive a round trip', () => {
    const params = new URLSearchParams('company=acme&topic=career&language=en&seniority=senior&q=jane&no_reviews=1');
    const forwarded = mentorFiltersToParams(mentorFiltersFromParams(params));
    expect(forwarded.get('company')).toBe('acme');
    expect(forwarded.get('topic')).toBe('career');
    expect(forwarded.get('language')).toBe('en');
    expect(forwarded.get('seniority')).toBe('senior');
    expect(forwarded.get('q')).toBe('jane');
    expect(forwarded.get('no_reviews')).toBe('1');
  });

  // no_reviews is a flag, not a value — it is either present as "1" or absent, unlike the
  // five string filters above.
  test('no_reviews is a flag: absent means unfiltered, never sent as false', () => {
    expect(mentorFiltersFromParams(new URLSearchParams('')).noReviews).toBe(false);
    expect(mentorFiltersToParams(emptyMentorFilters()).has('no_reviews')).toBe(false);
    expect(mentorFiltersToParams({ ...emptyMentorFilters(), noReviews: true }).get('no_reviews')).toBe('1');
  });

  // The directory's vocabulary is company/topic/language and nothing else — the same
  // list `knownMentorParams` holds in internal/api/handler/mentorship.go. Forwarding a
  // param the endpoint does not read would widen the answer while the address bar still
  // claims it was narrowed.
  test('a param the directory does not read never reaches the API', () => {
    const params = new URLSearchParams('company=acme&conutry=de&page=3');
    const forwarded = mentorFiltersToParams(mentorFiltersFromParams(params));
    expect(forwarded.get('company')).toBe('acme');
    expect(forwarded.has('conutry')).toBe(false);
    expect(forwarded.has('page')).toBe(false);
  });

  // Each filter is single-valued on the wire (`query.Get`, not `QueryAll`), so a link
  // carrying the same key twice must pick one rather than send both and let the backend
  // silently keep the first.
  test('a repeated key collapses to one value', () => {
    const params = new URLSearchParams('topic=career&topic=system-design');
    expect(mentorFiltersToParams(mentorFiltersFromParams(params)).getAll('topic')).toEqual(['career']);
  });

  // An empty value is not a filter. `?company=` narrows nothing, and forwarding it makes
  // "no company chosen" and "a company whose slug is the empty string" the same request.
  test('an empty value is dropped rather than forwarded', () => {
    const params = new URLSearchParams('company=&topic=career');
    const forwarded = mentorFiltersToParams(mentorFiltersFromParams(params));
    expect(forwarded.has('company')).toBe(false);
    expect(forwarded.get('topic')).toBe('career');
  });

  test('the query string form clears to empty, so a cleared filter has no trailing ?', () => {
    expect(mentorFiltersToQuery(emptyMentorFilters())).toBe('');
    expect(mentorFiltersToQuery({ ...emptyMentorFilters(), company: 'acme' })).toBe('company=acme');
  });

  // Surrounding space comes from hand-edited and shared links; it is not part of a slug.
  test('surrounding whitespace is trimmed away', () => {
    const params = new URLSearchParams('company=%20acme%20');
    expect(mentorFiltersToParams(mentorFiltersFromParams(params)).get('company')).toBe('acme');
  });
});

describe('seniorityLabel', () => {
  test('sentence-cases a value with no special label', () => {
    expect(seniorityLabel('senior')).toBe('Senior');
  });

  test('uses the shared label map for the one value that needs it', () => {
    expect(seniorityLabel('c_level')).toBe('C-level');
  });
});

describe('companyLabel', () => {
  test('prefers the display name when present', () => {
    expect(companyLabel('Acme Corp', 'acme')).toBe('Acme Corp');
  });

  test('falls back to the slug when only that is known', () => {
    expect(companyLabel('', 'acme')).toBe('acme');
  });

  test('reads as Independent when neither is set', () => {
    expect(companyLabel('', '')).toBe('Independent');
  });
});

describe('mentor filter options', () => {
  test('each distinct value is offered once', () => {
    const options = mentorFilterOptions([
      mentor({ topics: ['career', 'system-design'], languages: ['en', 'de'] }),
      mentor({ slug: 'bo', topics: ['career'], languages: ['en'] }),
    ]);
    expect(options.topics).toEqual(['career', 'system-design']);
    expect(options.languages).toEqual(['de', 'en']);
    expect(options.companies).toEqual([{ slug: 'acme', name: 'Acme' }]);
  });

  // The order has to come from the data rather than from whatever order the directory
  // happened to return, or the same two mentors reorder the controls between reloads and
  // the option a visitor was aiming at moves under the cursor.
  test('options are ordered, not left in arrival order', () => {
    const options = mentorFilterOptions([
      mentor({ topics: ['system-design'] }),
      mentor({ slug: 'bo', topics: ['career'] }),
    ]);
    expect(options.topics).toEqual(['career', 'system-design']);
  });

  // A company is offered by its slug (what the filter sends) carrying its name (what a
  // person reads). Two mentors at one employer are one option, not two.
  test('one employer with two mentors is one option', () => {
    const options = mentorFilterOptions([
      mentor(),
      mentor({ slug: 'bo', company_slug: 'acme', company_name: 'Acme' }),
      mentor({ slug: 'cy', company_slug: 'globex', company_name: 'Globex' }),
    ]);
    expect(options.companies).toEqual([
      { slug: 'acme', name: 'Acme' },
      { slug: 'globex', name: 'Globex' },
    ]);
  });

  test('a mentor listing no topics contributes none', () => {
    expect(mentorFilterOptions([mentor({ topics: [] })]).topics).toEqual([]);
  });

  test('an empty directory offers no options at all', () => {
    expect(mentorFilterOptions([])).toEqual({
      companies: [],
      topics: [],
      languages: [],
      seniorities: [],
    });
  });

  // Seniority is optional on a mentor, unlike topics/languages which are always arrays —
  // a mentor who left it unset must not turn into a spurious "" option nobody could have
  // meant to pick.
  test('seniority options are derived the same way as topics, and unset ones are skipped', () => {
    // 'lead' sorts before 'middle' alphabetically but after it in career order — picked
    // deliberately so a naive alpha sort would fail this assertion.
    const options = mentorFilterOptions([
      mentor({ seniority: 'lead' }),
      mentor({ slug: 'bo', seniority: 'middle' }),
      mentor({ slug: 'cy' }),
    ]);
    expect(options.seniorities).toEqual(['middle', 'lead']);
  });
});

describe('showing a booked session in the viewer zone', () => {
  // A booking carries only the absolute instant — no `local_start`, unlike a slot. So here
  // the zone conversion has to happen in the browser, and that is NOT a contradiction of
  // the slot rule: an instant ending in Z is unambiguous, while a slot's label was already
  // resolved server-side and converting it a second time is what moves it.
  test('the same booking reads differently in two zones', () => {
    expect(formatInstantIn('2026-10-15T16:00:00Z', 'America/Sao_Paulo')).toEqual({
      day: '2026-10-15',
      time: '13:00',
      offset: '-03:00',
    });
    expect(formatInstantIn('2026-10-15T16:00:00Z', 'Asia/Tokyo')).toEqual({
      day: '2026-10-16',
      time: '01:00',
      offset: '+09:00',
    });
  });

  test('UTC is offset zero, written out rather than blank', () => {
    expect(formatInstantIn('2026-10-15T16:00:00Z', 'UTC')).toEqual({
      day: '2026-10-15',
      time: '16:00',
      offset: '+00:00',
    });
  });

  // Midnight is the hour a 12-hour clock renders as "12" and a broken conversion as "24".
  test('midnight is 00:00', () => {
    expect(formatInstantIn('2026-10-15T00:00:00Z', 'UTC').time).toBe('00:00');
  });
});

describe('whether a session can still be cancelled', () => {
  const now = new Date('2026-10-15T12:00:00Z');

  test('a confirmed session that has not started can be cancelled', () => {
    expect(isCancellable(session(), now)).toBe(true);
  });

  // The backend refuses both of these; the button is hidden so nobody presses a control
  // that only ever returns an error.
  test('a session whose start has passed cannot', () => {
    expect(isCancellable(session({ starts_at: '2026-10-15T11:00:00Z' }), now)).toBe(false);
  });

  test('an already-cancelled session cannot', () => {
    expect(isCancellable(session({ status: 'cancelled' }), now)).toBe(false);
  });

  // Exactly at the start instant. The rule is "before its start", so the boundary is out —
  // and picking a side deliberately is the point, since this is the one moment where the
  // button and the endpoint could disagree.
  test('the start instant itself is too late', () => {
    expect(isCancellable(session({ starts_at: '2026-10-15T12:00:00Z' }), now)).toBe(false);
  });
});

describe('who may leave a review', () => {
  // Only the SEEKER of a completed session. The wire never says which party is reading:
  // what it says is that the seeker's address reaches the MENTOR alone, so its presence
  // is how the two are told apart. Indirect, and worth a test for exactly that reason.
  test('the seeker of a finished session may', () => {
    expect(canReview(session({ completed: true }))).toBe(true);
  });

  test('the mentor of the same session may not', () => {
    expect(canReview(session({ completed: true, seeker_email: 'seeker@example.test' }))).toBe(false);
  });

  // `completed` is already "confirmed AND ended", so a cancelled session is never
  // completed — the check does not need to repeat the status, and this pins that down.
  test('a session that has not finished may not be reviewed', () => {
    expect(canReview(session({ completed: false }))).toBe(false);
  });

  test('a cancelled session may not', () => {
    expect(canReview(session({ status: 'cancelled', completed: false }))).toBe(false);
  });
});

describe('weekdays, stored and displayed', () => {
  // The one place the two conventions meet. Storage is Go's time.Weekday, where 0 is
  // SUNDAY; the calendar draws Monday first. Getting this backwards shifts a mentor's
  // whole week by a day and looks like they typed it wrong.
  test('storage numbers name the right days', () => {
    expect(weekdayLabel(0)).toBe('Sunday');
    expect(weekdayLabel(1)).toBe('Monday');
    expect(weekdayLabel(6)).toBe('Saturday');
  });

  test('the display order runs Monday to Sunday, in storage numbers', () => {
    expect(weekdayOrder()).toEqual([1, 2, 3, 4, 5, 6, 0]);
  });

  // The calendar's column headings come from here rather than from a second list in the
  // component. This asserts the headings a Monday-first grid actually draws, so the two
  // cannot drift into disagreeing about which column is which.
  test('the short labels spell the calendar header', () => {
    expect(weekdayOrder().map(weekdayShortLabel)).toEqual(['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']);
  });

  // The two together: reading the display order through the labels must spell the week,
  // which is the assertion that fails if either side is fixed without the other.
  test('the order and the labels agree', () => {
    expect(weekdayOrder().map(weekdayLabel)).toEqual([
      'Monday',
      'Tuesday',
      'Wednesday',
      'Thursday',
      'Friday',
      'Saturday',
      'Sunday',
    ]);
  });
});

describe('splitting availability into a week and its exceptions', () => {
  test('a rule is weekly or dated, and lands in exactly one half', () => {
    const split = splitAvailability([rule({ id: 1, weekday: 1 }), rule({ id: 2, weekday: null, date: '2026-10-16' })]);
    expect(split.weekly.map((r) => r.id)).toEqual([1]);
    expect(split.dated.map((r) => r.id)).toEqual([2]);
  });

  // Weekly rules come back in the DISPLAY order, so a mentor reads their week as a week.
  test('weekly rules are ordered Monday first, then by start time', () => {
    const split = splitAvailability([
      rule({ id: 1, weekday: 0, start: '09:00' }),
      rule({ id: 2, weekday: 1, start: '18:00' }),
      rule({ id: 3, weekday: 1, start: '09:00' }),
    ]);
    expect(split.weekly.map((r) => r.id)).toEqual([3, 2, 1]);
  });

  test('dated rules are ordered by date', () => {
    const split = splitAvailability([
      rule({ id: 1, weekday: null, date: '2026-11-02' }),
      rule({ id: 2, weekday: null, date: '2026-10-16' }),
    ]);
    expect(split.dated.map((r) => r.id)).toEqual([2, 1]);
  });

  test('an empty schedule splits into two empty halves', () => {
    expect(splitAvailability([])).toEqual({ weekly: [], dated: [] });
  });
});

describe('writing the booker state back to the query', () => {
  test('a set value is written and the rest is left alone', () => {
    const current = new URLSearchParams('month=2026-10&ref=newsletter');
    expect(withSearchParams(current, { date: '2026-10-15' })).toBe('month=2026-10&ref=newsletter&date=2026-10-15');
  });

  // Picking a new day has to DROP the slot chosen under the old one, or the page shows one
  // date while the button is armed with an hour from another.
  test('a null value removes its key', () => {
    const current = new URLSearchParams('month=2026-10&date=2026-10-15&slot=x');
    expect(withSearchParams(current, { date: '2026-10-16', slot: null })).toBe('month=2026-10&date=2026-10-16');
  });

  test('an empty string removes its key too, so a cleared control does not linger', () => {
    const current = new URLSearchParams('company=acme');
    expect(withSearchParams(current, { company: '' })).toBe('');
  });

  // The source must not be mutated: it is `page.url.searchParams`, and writing through it
  // edits the address bar's own object behind the router's back.
  test('the params it was given are left untouched', () => {
    const current = new URLSearchParams('month=2026-10');
    withSearchParams(current, { date: '2026-10-15' });
    expect(current.toString()).toBe('month=2026-10');
  });
});

describe('seedFormFromSuggestions', () => {
  function blankForm(): MentorProfileInput {
    return {
      company_slug: '',
      slug: '',
      name: '',
      headline: '',
      bio: '',
      topics: [],
      languages: [],
      timezone: 'Europe/Amsterdam',
      session_minutes: 60,
      buffer_before_minutes: 0,
      buffer_after_minutes: 15,
      notice_minutes: 120,
      horizon_days: 30,
      meeting_url: '',
      show_photo: false,
      seniority: '',
    };
  }

  test('a present field overrides the blank default', () => {
    const suggestions: MentorProfileSuggestions = {
      name: 'Jane Doe',
      headline: 'Staff Engineer',
      bio: 'Ten years of Go.',
      timezone: 'Europe/Berlin',
      company_slug: 'acme',
      topics: ['backend', 'career'],
      languages: ['English', 'German'],
    };
    const form = seedFormFromSuggestions(blankForm(), suggestions);

    expect(form.name).toBe('Jane Doe');
    expect(form.headline).toBe('Staff Engineer');
    expect(form.bio).toBe('Ten years of Go.');
    expect(form.timezone).toBe('Europe/Berlin');
    expect(form.company_slug).toBe('acme');
    expect(form.topics).toEqual(['backend', 'career']);
    expect(form.languages).toEqual(['English', 'German']);
  });

  test('an absent field keeps the blank default, including the browser timezone', () => {
    const form = seedFormFromSuggestions(blankForm(), {});

    expect(form.name).toBe('');
    expect(form.company_slug).toBe('');
    expect(form.timezone).toBe('Europe/Amsterdam');
    expect(form.topics).toEqual([]);
    expect(form.languages).toEqual([]);
  });

  test('every other field is left exactly as the blank form set it', () => {
    const form = seedFormFromSuggestions(blankForm(), { name: 'Jane Doe' });

    expect(form.session_minutes).toBe(60);
    expect(form.buffer_after_minutes).toBe(15);
    expect(form.notice_minutes).toBe(120);
    expect(form.horizon_days).toBe(30);
    expect(form.meeting_url).toBe('');
    expect(form.slug).toBe('');
  });
});

describe('profileInputFromProfile', () => {
  function ownProfile(over: Partial<OwnMentorProfile> = {}): OwnMentorProfile {
    return {
      slug: 'jane-doe',
      name: 'Jane Doe',
      company_slug: 'acme',
      company_name: 'Acme',
      headline: 'Staff Engineer',
      bio: 'Ten years of Go.',
      topics: ['backend'],
      languages: ['English'],
      timezone: 'Europe/Berlin',
      session_minutes: 45,
      rating_count: 3,
      rating_avg: 4.7,
      show_photo: true,
      status: 'approved',
      paused: false,
      created_at: '2026-09-01T00:00:00Z',
      meeting_url: 'https://meet.example.test/jane',
      buffer_before_minutes: 5,
      buffer_after_minutes: 10,
      notice_minutes: 60,
      horizon_days: 14,
      ...over,
    };
  }

  test('every editable field carries over unchanged', () => {
    const input = profileInputFromProfile(ownProfile());

    expect(input).toEqual({
      company_slug: 'acme',
      slug: 'jane-doe',
      name: 'Jane Doe',
      headline: 'Staff Engineer',
      bio: 'Ten years of Go.',
      topics: ['backend'],
      languages: ['English'],
      timezone: 'Europe/Berlin',
      session_minutes: 45,
      buffer_before_minutes: 5,
      buffer_after_minutes: 10,
      notice_minutes: 60,
      horizon_days: 14,
      meeting_url: 'https://meet.example.test/jane',
      show_photo: true,
      seniority: '',
    });
  });

  test('a stated seniority carries over unchanged', () => {
    expect(profileInputFromProfile(ownProfile({ seniority: 'senior' })).seniority).toBe('senior');
  });

  test('an unset seniority becomes the empty string, never undefined', () => {
    expect(profileInputFromProfile(ownProfile({ seniority: undefined })).seniority).toBe('');
  });
});

function interval(startsAt: string, endsAt: string, status: MentorCalendarInterval['status']): MentorCalendarInterval {
  return { starts_at: startsAt, ends_at: endsAt, status };
}

describe('grouping a mentor own-calendar into local days', () => {
  test('an interval wholly inside one day is grouped under it alone', () => {
    const grouped = groupCalendarByLocalDay([interval('2026-09-08T16:00:00Z', '2026-09-08T17:00:00Z', 'free')], 'UTC');
    expect([...grouped.keys()]).toEqual(['2026-09-08']);
  });

  // The whole reason this differs from groupSlotsByLocalDay: a mentor with no
  // availability rules yet gets back ONE closed interval spanning the entire requested
  // window, not one per day. Bucketing only by start day would leave every day after the
  // first showing no data at all, when the source data says they are closed too.
  test('an interval spanning several days appears under every day it touches', () => {
    const grouped = groupCalendarByLocalDay(
      [interval('2026-09-01T00:00:00Z', '2026-09-04T00:00:00Z', 'closed')],
      'UTC',
    );
    expect([...grouped.keys()]).toEqual(['2026-09-01', '2026-09-02', '2026-09-03']);
  });

  // ends_at is exclusive, matching the domain's half-open intervals: an interval ending
  // exactly at a day's start does not touch that day.
  test('an interval ending exactly at midnight does not touch that day', () => {
    const grouped = groupCalendarByLocalDay([interval('2026-09-08T22:00:00Z', '2026-09-09T00:00:00Z', 'busy')], 'UTC');
    expect([...grouped.keys()]).toEqual(['2026-09-08']);
  });

  test('grouping is in the given timezone, not UTC', () => {
    // 22:00 UTC is already the 9th in Europe/Berlin (UTC+2 in September).
    const grouped = groupCalendarByLocalDay(
      [interval('2026-09-08T22:00:00Z', '2026-09-08T23:00:00Z', 'free')],
      'Europe/Berlin',
    );
    expect([...grouped.keys()]).toEqual(['2026-09-09']);
  });

  test('an empty interval list groups into nothing', () => {
    expect(groupCalendarByLocalDay([], 'UTC').size).toBe(0);
  });
});

describe('which statuses a day shows, in priority order', () => {
  test('booked outranks busy, free and closed', () => {
    const day = [
      interval('2026-09-08T09:00:00Z', '2026-09-08T10:00:00Z', 'closed'),
      interval('2026-09-08T18:00:00Z', '2026-09-08T19:00:00Z', 'free'),
      interval('2026-09-08T20:00:00Z', '2026-09-08T21:00:00Z', 'booked'),
    ];
    expect(dayStatuses(day)).toEqual(['booked', 'free', 'closed']);
  });

  test('a day with only closed time reports just closed', () => {
    expect(dayStatuses([interval('2026-09-08T00:00:00Z', '2026-09-09T00:00:00Z', 'closed')])).toEqual(['closed']);
  });

  test('a day with no intervals reports nothing', () => {
    expect(dayStatuses([])).toEqual([]);
  });
});

describe('the weekly schedule editor', () => {
  const slotRule = (id: number, weekday: number, start: string, end: string): MentorAvailabilityRule => ({
    id,
    weekday,
    date: null,
    start,
    end,
    closure: false,
  });

  test('draws all seven days Monday first, each with its own ranges in order', () => {
    const draft = weekDraft([
      slotRule(1, 0, '10:00', '12:00'),
      slotRule(2, 2, '19:00', '21:00'),
      slotRule(3, 2, '08:00', '09:00'),
    ]);
    expect(draft.map((d) => d.weekday)).toEqual([1, 2, 3, 4, 5, 6, 0]);
    expect(draft[1]?.ranges).toEqual([
      { start: '08:00', end: '09:00' },
      { start: '19:00', end: '21:00' },
    ]);
    expect(draft[6]?.ranges).toEqual([{ start: '10:00', end: '12:00' }]);
    expect(draft[0]?.ranges).toEqual([]);
  });

  test('flattens back into the rows the replace-week endpoint takes', () => {
    const rows = weekRows(weekDraft([slotRule(1, 0, '10:00', '12:00'), slotRule(2, 1, '18:00', '20:00')]));
    expect(rows).toEqual([
      { weekday: 1, start: '18:00', end: '20:00' },
      { weekday: 0, start: '10:00', end: '12:00' },
    ]);
  });

  test('a new range follows the last one, and never wraps past midnight', () => {
    expect(nextWeekRange([])).toEqual({ start: '18:00', end: '20:00' });
    expect(nextWeekRange([{ start: '09:00', end: '11:30' }])).toEqual({ start: '11:30', end: '13:30' });
    expect(nextWeekRange([{ start: '20:00', end: '23:00' }])).toEqual({ start: '18:00', end: '20:00' });
  });

  test('an exception names its day without reading the date in a zone', () => {
    expect(exceptionDayLabel('2026-11-04')).toBe('Wed 4 Nov 2026');
    expect(exceptionDayLabel('nonsense')).toBe('nonsense');
  });
});

describe('the month a booking calendar opens on', () => {
  test('shares opening request bounds across visitors throughout a UTC day', () => {
    const window = { from: '2027-01-31T00:00:00.000Z', to: '2027-04-03T00:00:00.000Z' };
    expect(openingSlotWindow(new Date('2027-01-31T12:00:01Z'))).toEqual(window);
    expect(openingSlotWindow(new Date('2027-01-31T23:59:59Z'))).toEqual(window);
  });

  test('the opening read searches forward within the endpoint limit across short months and year boundaries', () => {
    expect(openingSlotWindow(new Date('2027-01-31T12:00:00Z'))).toEqual({
      from: '2027-01-31T00:00:00.000Z',
      to: '2027-04-03T00:00:00.000Z',
    });
    expect(openingSlotWindow(new Date('2026-12-31T12:00:00Z'))).toEqual({
      from: '2026-12-31T00:00:00.000Z',
      to: '2027-03-03T00:00:00.000Z',
    });
  });

  test('stays on the shown month while it has an open hour', () => {
    expect(firstOpenMonth([slot('2026-11-02T19:00:00+01:00'), slot('2026-10-31T19:00:00+01:00')], '2026-10')).toBe(
      '2026-10',
    );
  });

  test('moves to the month of the earliest open hour when the shown month has none', () => {
    expect(firstOpenMonth([slot('2026-12-01T09:00:00+01:00'), slot('2026-11-02T19:00:00+01:00')], '2026-10')).toBe(
      '2026-11',
    );
  });

  test('ignores the padding day before the shown month and stays put with nothing open', () => {
    expect(firstOpenMonth([slot('2026-09-30T22:00:00+01:00')], '2026-10')).toBe('2026-10');
    expect(firstOpenMonth([], '2026-10')).toBe('2026-10');
  });
});
