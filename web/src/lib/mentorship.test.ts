import { describe, expect, test } from 'vitest';
import {
  addMonths,
  daysInMonth,
  daysWithSlots,
  emptyMentorFilters,
  monthGrid,
  monthOf,
  slotWindowForMonth,
  todayIn,
  withSearchParams,
  groupSlotsByLocalDay,
  mentorFilterOptions,
  mentorFiltersFromParams,
  mentorFiltersToParams,
  mentorFiltersToQuery,
  slotLocalDay,
  slotLocalTime,
} from './mentorship';
import { formatInstantIn, isCancellable } from './mentorship';
import type { Mentor, MentorSession, MentorSlot } from './types';

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

function slot(localStart: string, utcOffset = '+00:00'): MentorSlot {
  return {
    starts_at: '2026-10-15T09:00:00Z',
    ends_at: '2026-10-15T10:00:00Z',
    local_start: localStart,
    local_end: localStart,
    utc_offset: utcOffset,
  };
}

function mentor(over: Partial<Mentor> = {}): Mentor {
  return {
    slug: 'anna-k',
    name: 'Anna K.',
    company_slug: 'acme',
    company_name: 'Acme',
    headline: 'Staff Engineer',
    bio: '',
    topics: ['career'],
    languages: ['en'],
    timezone: 'Europe/Berlin',
    session_minutes: 60,
    rating_count: 0,
    rating_avg: 0,
    ...over,
  };
}

describe('mentor directory filters', () => {
  test('empty filters serialize to an empty query', () => {
    expect(mentorFiltersToParams(emptyMentorFilters()).toString()).toBe('');
  });

  test('the three filters the directory reads survive a round trip', () => {
    const params = new URLSearchParams('company=acme&topic=career&language=en');
    expect(mentorFiltersToParams(mentorFiltersFromParams(params)).toString()).toBe(
      new URLSearchParams({ company: 'acme', topic: 'career', language: 'en' }).toString(),
    );
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
    expect(mentorFiltersToParams(mentorFiltersFromParams(params)).getAll('topic')).toEqual([
      'career',
    ]);
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
    expect(mentorFiltersToQuery({ company: 'acme', topic: '', language: '' })).toBe(
      'company=acme',
    );
  });

  // Surrounding space comes from hand-edited and shared links; it is not part of a slug.
  test('surrounding whitespace is trimmed away', () => {
    const params = new URLSearchParams('company=%20acme%20');
    expect(mentorFiltersToParams(mentorFiltersFromParams(params)).get('company')).toBe('acme');
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
    expect(mentorFilterOptions([])).toEqual({ companies: [], topics: [], languages: [] });
  });
});

describe('reading a slot in the viewer zone', () => {
  // THE trap this whole module exists to avoid. `local_start` is already in the viewer's
  // zone, so its own date IS the day it belongs to. Route it through a Date and the value
  // is re-interpreted against whatever zone the browser is in: for Tokyo's 16th at 01:00
  // the instant is still the 15th in UTC, and a naive
  // `new Date(local_start).toISOString().slice(0, 10)` files the slot under the wrong day.
  test('the local day is read from the label, not recomputed from the instant', () => {
    expect(slotLocalDay(slot('2026-10-16T01:00:00+09:00'))).toBe('2026-10-16');
    // Proof the naive route really does disagree, so this test is guarding something.
    expect(new Date('2026-10-16T01:00:00+09:00').toISOString().slice(0, 10)).toBe('2026-10-15');
  });

  test('the wall clock is read from the label too', () => {
    expect(slotLocalTime(slot('2026-10-16T01:30:00+09:00'))).toBe('01:30');
  });

  test('a zoneless label is still read as written', () => {
    expect(slotLocalDay(slot('2026-03-29T02:00:00Z'))).toBe('2026-03-29');
    expect(slotLocalTime(slot('2026-03-29T02:00:00Z'))).toBe('02:00');
  });
});

describe('grouping slots into calendar days', () => {
  test('slots are grouped under their own local day, in order', () => {
    const grouped = groupSlotsByLocalDay([
      slot('2026-10-15T11:00:00+03:00'),
      slot('2026-10-16T09:00:00+03:00'),
      slot('2026-10-15T17:00:00+03:00'),
    ]);
    expect([...grouped.keys()]).toEqual(['2026-10-15', '2026-10-16']);
    expect(grouped.get('2026-10-15')?.length).toBe(2);
  });

  // The autumn transition repeats an hour, so two DIFFERENT instants carry the same
  // wall-clock label and differ only by their offset. Both are real hours the mentor
  // stated; collapsing them would silently withhold one, and the spec says neither is.
  test('the repeated autumn hour keeps both slots', () => {
    const grouped = groupSlotsByLocalDay([
      slot('2026-10-25T02:00:00+03:00', '+03:00'),
      slot('2026-10-25T02:00:00+02:00', '+02:00'),
    ]);
    expect(grouped.get('2026-10-25')?.length).toBe(2);
  });

  test('an empty slot list groups into nothing', () => {
    expect(groupSlotsByLocalDay([]).size).toBe(0);
  });

  // What the month grid greys out. Derived from the same grouping rather than measured
  // separately, so a day can never be enabled in the grid and empty when opened.
  test('the days with slots are exactly the grouped keys', () => {
    const slots = [slot('2026-10-15T11:00:00+03:00'), slot('2026-10-20T11:00:00+03:00')];
    expect(daysWithSlots(slots)).toEqual(new Set(['2026-10-15', '2026-10-20']));
  });
});

describe('the month a day belongs to', () => {
  test('a day names its own month', () => {
    expect(monthOf('2026-10-15')).toBe('2026-10');
  });

  test('stepping forward rolls the year over', () => {
    expect(addMonths('2026-11', 1)).toBe('2026-12');
    expect(addMonths('2026-12', 1)).toBe('2027-01');
  });

  test('stepping back rolls the year under', () => {
    expect(addMonths('2026-01', -1)).toBe('2025-12');
  });

  test('the month is always two digits', () => {
    expect(addMonths('2026-08', 1)).toBe('2026-09');
  });
});

describe('days in a month', () => {
  test('the ordinary lengths', () => {
    expect(daysInMonth('2026-01')).toBe(31);
    expect(daysInMonth('2026-04')).toBe(30);
    expect(daysInMonth('2026-02')).toBe(28);
  });

  // The three leap rules, each with a case that only it decides. 2100 is the one a bare
  // "divisible by four" gets wrong, and it is inside the horizon of nothing — but the rule
  // costs one character and being wrong about it is the kind of thing nobody re-derives.
  test('the leap-year rules', () => {
    expect(daysInMonth('2028-02')).toBe(29);
    expect(daysInMonth('2000-02')).toBe(29);
    expect(daysInMonth('2100-02')).toBe(28);
  });
});

describe('the month grid', () => {
  // Monday-first, which is what the approved layout shows and what most of this
  // catalogue's audience reads.
  test('a month starting mid-week is padded to Monday', () => {
    // 2026-10-01 is a Thursday, so the first row carries three blanks.
    const weeks = monthGrid('2026-10');
    expect(weeks[0]?.slice(0, 4)).toEqual(['', '', '', '2026-10-01']);
  });

  test('a month starting on a Monday needs no padding', () => {
    // 2026-06-01 is a Monday.
    expect(monthGrid('2026-06')[0]?.[0]).toBe('2026-06-01');
  });

  test('every row is a full week and every day appears once', () => {
    const weeks = monthGrid('2026-10');
    for (const week of weeks) expect(week.length).toBe(7);
    const days = weeks.flat().filter(Boolean);
    expect(days.length).toBe(31);
    expect(new Set(days).size).toBe(31);
    expect(days[0]).toBe('2026-10-01');
    expect(days[30]).toBe('2026-10-31');
  });

  // The worst case for a Monday-first grid: 2026-02-01 is a Sunday, so it lands in the
  // LAST column and the first row is six blanks. A Sunday-first grid would have needed
  // none, which is exactly why the convention has to be decided once and tested.
  test('a month starting on a Sunday is padded by six', () => {
    const weeks = monthGrid('2026-02');
    expect(weeks[0]).toEqual(['', '', '', '', '', '', '2026-02-01']);
    expect(weeks.flat().filter(Boolean).length).toBe(28);
    expect(weeks.at(-1)?.some((d) => d !== '')).toBe(true);
  });
});

describe('today, in the zone that was actually used', () => {
  // "Today" is a question about a zone, not about a machine. At 16:00 UTC it is already
  // tomorrow in Tokyo, so a calendar that opens on the server's day opens on the wrong
  // month for a seeker there for eight hours out of every twenty-four.
  test('the same instant is a different day in two zones', () => {
    const instant = new Date('2026-10-15T16:00:00Z');
    expect(todayIn('Asia/Tokyo', instant)).toBe('2026-10-16');
    expect(todayIn('UTC', instant)).toBe('2026-10-15');
    expect(todayIn('America/Los_Angeles', instant)).toBe('2026-10-15');
  });

  // Just past midnight UTC is still the previous day west of it — the mirror case, so a
  // wrong sign in the conversion cannot pass both.
  test('the day before midnight UTC still reads as yesterday in the west', () => {
    const instant = new Date('2026-10-15T00:30:00Z');
    expect(todayIn('America/Los_Angeles', instant)).toBe('2026-10-14');
    expect(todayIn('UTC', instant)).toBe('2026-10-15');
  });
});

describe('the slot window a month needs', () => {
  // The endpoint takes INSTANTS and answers in the viewer's zone, so a window cut exactly
  // on UTC month boundaries loses the month's first local morning east of UTC and its last
  // local evening west of it. A day of padding on each side costs nothing and closes both.
  test('the window covers the month with a day of padding on each side', () => {
    expect(slotWindowForMonth('2026-10')).toEqual({
      from: '2026-09-30T00:00:00Z',
      to: '2026-11-01T00:00:00Z',
    });
  });

  test('padding rolls across a year boundary', () => {
    expect(slotWindowForMonth('2026-01')).toEqual({
      from: '2025-12-31T00:00:00Z',
      to: '2026-02-01T00:00:00Z',
    });
  });

  test('padding lands on the right day after a leap February', () => {
    expect(slotWindowForMonth('2028-03').from).toBe('2028-02-29T00:00:00Z');
  });

  // The endpoint refuses a span wider than 62 days; a padded month is at most 33.
  test('the window never exceeds what the endpoint will compute', () => {
    for (const month of ['2026-01', '2026-02', '2028-02', '2026-04']) {
      const { from, to } = slotWindowForMonth(month);
      const days = (Date.parse(to) - Date.parse(from)) / 86_400_000;
      expect(days).toBeLessThanOrEqual(62);
    }
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

describe('writing the booker state back to the query', () => {
  test('a set value is written and the rest is left alone', () => {
    const current = new URLSearchParams('month=2026-10&ref=newsletter');
    expect(withSearchParams(current, { date: '2026-10-15' })).toBe(
      'month=2026-10&ref=newsletter&date=2026-10-15',
    );
  });

  // Picking a new day has to DROP the slot chosen under the old one, or the page shows one
  // date while the button is armed with an hour from another.
  test('a null value removes its key', () => {
    const current = new URLSearchParams('month=2026-10&date=2026-10-15&slot=x');
    expect(withSearchParams(current, { date: '2026-10-16', slot: null })).toBe(
      'month=2026-10&date=2026-10-16',
    );
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
