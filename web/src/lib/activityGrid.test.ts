// The reader's timezone decides which square an event lands on, so the suite runs in one
// that is not UTC — and `activityGrid.west.test.ts` mirrors it on the other side of
// Greenwich. Set before anything touches Date, for the reason calendarModel.test.ts gives.
process.env.TZ = 'Europe/Warsaw'; // UTC+2 in summer

import { describe, expect, it } from 'vitest';

import { APPLICATION_EVENT_KINDS, APPLICATION_EVENT_SOURCES } from './generated/contracts';
import {
  buildActivityGrid,
  countsAsCandidateAction,
  KIND_VERDICT,
  LEVELS,
  levelFor,
  rangeForWindow,
  SOURCE_IS_THE_CANDIDATE,
  WINDOW_DAYS,
} from './activityGrid';
import type { TimelineEvent } from './types';

const event = (kind: string, source: string): TimelineEvent =>
  ({ id: 1, kind, source, observed: true, occurred_at: '2026-08-01T09:00:00Z', company_slug: 'acme' }) as TimelineEvent;

/** An event at local midday on a calendar date, so the instant is unambiguous whatever the
 *  zone — the boundary cases below set their instants explicitly instead. */
const on = (day: string, kind = 'applied', source = 'user'): TimelineEvent => {
  const [y = 0, m = 1, d = 1] = day.split('-').map(Number);
  return {
    id: 1,
    kind,
    source,
    observed: true,
    occurred_at: new Date(y, m - 1, d, 12, 0, 0).toISOString(),
    company_slug: 'acme',
  } as TimelineEvent;
};

const TODAY = new Date(2026, 8, 16); // 16 September 2026, local

// What a square measures is EFFORT, and the ledger records more than effort. Measured on
// production over the year to 2026-09-16, 30% of events were nobody's effort but the
// platform's or an employer's — 424 `stage_set` from `system` (a listing noticed closed)
// and 205 `employer_reply`. Shading a square for those credits the candidate with a day
// they did not work, which is the one thing a streak must never do.
describe('countsAsCandidateAction', () => {
  it('counts an application however it was submitted', () => {
    for (const source of ['user', 'assistant', 'auto_apply']) {
      expect(countsAsCandidateAction(event('applied', source)), source).toBe(true);
    }
  });

  it('counts a follow-up', () => {
    expect(countsAsCandidateAction(event('follow_up_sent', 'user'))).toBe(true);
  });

  // `stage_set` is the only kind both the candidate and the platform produce, so it is the
  // only one where the source decides.
  it('counts a stage the candidate moved', () => {
    for (const source of ['user', 'assistant', 'auto_apply']) {
      expect(countsAsCandidateAction(event('stage_set', source)), source).toBe(true);
    }
  });

  it('does not count a stage the platform moved on its own', () => {
    expect(countsAsCandidateAction(event('stage_set', 'system'))).toBe(false);
  });

  // A stage derived from mail or calendar evidence is an observation of somebody else's
  // behaviour, dated by them. Reading the inbox is not an action.
  it('does not count a stage derived from observed evidence', () => {
    for (const source of ['mail_gmail', 'mail_hosted', 'mail_external', 'calendar_google']) {
      expect(countsAsCandidateAction(event('stage_set', source)), source).toBe(false);
    }
  });

  it('never counts an employer reply, whatever its source', () => {
    for (const source of ['mail_gmail', 'mail_hosted', 'mail_external', 'user']) {
      expect(countsAsCandidateAction(event('employer_reply', source)), source).toBe(false);
    }
  });

  // Getting an interview is an outcome, not a day's work — and the arranging is usually the
  // employer's. It belongs in the day panel, not in the shading.
  it('does not count an interview being scheduled', () => {
    expect(countsAsCandidateAction(event('interview_scheduled', 'calendar_google'))).toBe(false);
  });

  // The rule is a closed list, not a denylist. A denylist admits whatever the ledger gains
  // next, and what it is most likely to gain next is another observation of somebody else's
  // behaviour — which would inflate every square silently. A closed list fails the other way:
  // a genuine new candidate action goes UNCOUNTED, which is visible, and which the
  // exhaustiveness test below turns into a red build rather than a quiet undercount.
  it('does not count a kind from a newer server', () => {
    expect(countsAsCandidateAction(event('offer_withdrawn', 'user'))).toBe(false);
  });

  it('does not count a source from a newer server', () => {
    expect(countsAsCandidateAction(event('stage_set', 'mobile_app'))).toBe(false);
  });
});

// The rule above is only as good as its coverage of the vocabulary it judges. A kind or a
// source added in Go and forgotten here does not fail loudly — it scores zero, which is
// indistinguishable from a quiet day, and an undercounted streak is exactly the discouraging
// lie this feature must not tell. So the verdicts are held to the GENERATED vocabularies, the
// way events.test.ts holds its labels: adding `KindOfferAccepted` in Go turns this red until
// somebody states whether accepting an offer is a day's work.
describe('the verdicts cover the whole vocabulary', () => {
  it('states a verdict for every kind the contract carries', () => {
    for (const kind of APPLICATION_EVENT_KINDS) {
      expect(KIND_VERDICT[kind], kind).toBeDefined();
    }
  });

  it('states a verdict for every source the contract carries', () => {
    for (const source of APPLICATION_EVENT_SOURCES) {
      expect(SOURCE_IS_THE_CANDIDATE[source], source).toBeTypeOf('boolean');
    }
  });

  // The two tables must also agree with the predicate, or the exhaustiveness above would be
  // checking a table nothing reads.
  it('is what the predicate actually consults', () => {
    for (const kind of APPLICATION_EVENT_KINDS) {
      for (const source of APPLICATION_EVENT_SOURCES) {
        const verdict = KIND_VERDICT[kind];
        const expected = verdict === 'candidate' || (verdict === 'source-decides' && SOURCE_IS_THE_CANDIDATE[source]);
        expect(countsAsCandidateAction(event(kind, source)), `${kind}/${source}`).toBe(expected);
      }
    }
  });
});

// Measured on production: the median caller records 5 counting actions in a WHOLE YEAR, p95
// records 11, and the heaviest records 655. No absolute scale fits both ends — one calibrated
// for 655 draws almost every real grid as a uniform palest green, one calibrated for 5
// saturates the moment somebody bulk-applies. So the scale is relative, with a floor.
describe('levelFor', () => {
  it('draws a day with no actions as level zero', () => {
    expect(levelFor(0, 10)).toBe(0);
  });

  // Dividing by `busiest` naively puts every active day at the darkest shade for the person
  // whose best day was one application — a solid dark year that says nothing. Below the floor
  // the scale is the count itself, so one action reads as one.
  it('reads a small count as itself rather than as a share of a small maximum', () => {
    expect(levelFor(1, 1)).toBe(1);
    expect(levelFor(1, 3)).toBe(1);
    expect(levelFor(2, 3)).toBe(2);
    expect(levelFor(3, 3)).toBe(3);
  });

  it('caps the fixed steps at the darkest shade', () => {
    expect(levelFor(5, 6)).toBe(LEVELS);
    expect(levelFor(6, 6)).toBe(LEVELS);
  });

  // Once somebody's busiest day is large, fixed steps would put every ordinary day at the
  // darkest shade and the grid would stop distinguishing anything.
  it('stretches to quarters of the busiest day once that day is large', () => {
    expect(levelFor(1, 40)).toBe(1);
    expect(levelFor(10, 40)).toBe(1);
    expect(levelFor(20, 40)).toBe(2);
    expect(levelFor(30, 40)).toBe(3);
    expect(levelFor(40, 40)).toBe(LEVELS);
  });

  // The one rule that cannot bend: a real action drawn as a rest day would break, on screen,
  // a streak the person did not break.
  it('never draws a day that had an action as empty', () => {
    for (let busiest = 1; busiest <= 60; busiest++) {
      for (let count = 1; count <= busiest; count++) {
        const level = levelFor(count, busiest);
        expect(level, `${count} of ${busiest}`).toBeGreaterThanOrEqual(1);
        expect(level, `${count} of ${busiest}`).toBeLessThanOrEqual(LEVELS);
      }
    }
  });

  // `busiest` is derived from the same series, so it cannot really be below `count` — but a
  // scale that divides by it must not produce a level off the end of the palette if it ever is.
  it('stays inside the palette when the maximum is missing or wrong', () => {
    expect(levelFor(3, 0)).toBeGreaterThanOrEqual(1);
    expect(levelFor(3, 0)).toBeLessThanOrEqual(LEVELS);
    expect(levelFor(99, 40)).toBe(LEVELS);
  });
});

describe('buildActivityGrid shape', () => {
  it('runs whole weeks, so every row is seven cells', () => {
    const grid = buildActivityGrid([], TODAY);

    expect(grid.weeks.length).toBeGreaterThan(0);
    for (const week of grid.weeks) expect(week.days).toHaveLength(7);
    expect(grid.days).toHaveLength(grid.weeks.length * 7);
  });

  // The caption rail reads `monthStart` straight down the columns, so a month must be
  // introduced exactly once — a second label for the same month is a caption pointing at
  // nothing, and a missing one leaves a stretch of the year unnamed.
  it('introduces each month on exactly one column', () => {
    const grid = buildActivityGrid([], TODAY);
    const labelled = grid.weeks.flatMap((w) => (w.monthStart ? [w.monthStart.getMonth()] : []));

    expect(labelled.length).toBeGreaterThanOrEqual(12);
    for (let i = 1; i < labelled.length; i++) expect(labelled[i]).not.toBe(labelled[i - 1]);
  });

  it('keys every column distinctly', () => {
    const keys = buildActivityGrid([], TODAY).weeks.map((w) => w.key);

    expect(new Set(keys).size).toBe(keys.length);
  });

  it('ends on the reader own today and spans exactly the window', () => {
    const grid = buildActivityGrid([], TODAY);
    const inWindow = grid.days.filter((d) => d.inWindow);

    expect(inWindow).toHaveLength(WINDOW_DAYS);
    expect(inWindow.at(-1)?.key).toBe('2026-09-16');
    expect(inWindow.at(-1)?.isToday).toBe(true);
    expect(inWindow[0]?.key).toBe('2025-09-19'); // WINDOW_DAYS - 1 days before 2026-09-16
  });

  // The endpoint refuses a span over `apptimeline.MaxRangeDays` outright rather than trimming
  // it, so a window one hour too wide is not a smaller grid — it is an error state for every
  // reader on that day. And the Go check is `to.Sub(from) > 366*24h`, an ABSOLUTE duration:
  // a calendar day is 25 hours when the clocks go back, so a span of 366 calendar dates that
  // happens to contain two autumn transitions lasts 366 days and two hours and is refused.
  //
  // Checked on EVERY date of a year rather than on one, because that is exactly the shape of
  // the bug: picking a single day to assert on picks a day that passes. Two of them do not.
  it('asks for a span the endpoint will answer, on every day of the year', () => {
    const CAP_MS = 366 * 86_400_000;
    for (let i = 0; i < 366; i++) {
      const day = new Date(2026, 0, 1 + i);
      const { from, to } = rangeForWindow(day);
      expect(Date.parse(to) - Date.parse(from), day.toDateString()).toBeLessThanOrEqual(CAP_MS);
    }
  });

  it('still covers the whole window with a day of margin at each end', () => {
    const { from, to } = rangeForWindow(TODAY);
    const grid = buildActivityGrid([], TODAY);
    const firstSquare = grid.days.find((d) => d.inWindow);

    expect(firstSquare).toBeDefined();
    expect(Date.parse(from)).toBeLessThan(firstSquare?.date.getTime() ?? 0);
    expect(Date.parse(to)).toBeGreaterThan(new Date(2026, 8, 16, 23, 59).getTime());
  });

  // The cells before the window starts and after today exist only to keep the rows square.
  // Drawing them as level-zero days would claim we measured a day we did not.
  it('pads the first and last weeks with cells marked outside the window', () => {
    const grid = buildActivityGrid([], TODAY);
    const pad = grid.days.filter((d) => !d.inWindow);

    expect(pad.length).toBeGreaterThan(0);
    for (const cell of pad) expect(cell.count).toBe(0);
  });

  // A day with nothing on it is part of the answer — it is what a gap in a streak looks
  // like. Omitting it would make the grid shorter on a quiet year.
  it('keeps a day with no actions as a level-zero cell', () => {
    const grid = buildActivityGrid([on('2026-09-16')], TODAY);
    const quiet = grid.days.find((d) => d.key === '2026-09-15');

    expect(quiet).toBeDefined();
    expect(quiet?.count).toBe(0);
    expect(quiet?.level).toBe(0);
  });

  // Day stepping by calendar date rather than by 86_400_000 ms. Warsaw leaves summer time
  // on 25 October 2025, inside this window; a window stepped in milliseconds drifts an hour
  // and eventually repeats or skips a date.
  it('gives every cell its own consecutive calendar date across a clock change', () => {
    const grid = buildActivityGrid([], TODAY);
    const keys = grid.days.map((d) => d.key);

    expect(new Set(keys).size).toBe(keys.length);
    for (let i = 1; i < grid.days.length; i++) {
      const previous = grid.days[i - 1]?.date ?? new Date(0);
      const expected = new Date(previous.getFullYear(), previous.getMonth(), previous.getDate() + 1);
      const want = `${expected.getFullYear()}-${String(expected.getMonth() + 1).padStart(2, '0')}-${String(expected.getDate()).padStart(2, '0')}`;
      expect(grid.days[i]?.key, `cell ${i}`).toBe(want);
    }
  });
});

describe('buildActivityGrid files a day by the reader own clock', () => {
  // 22:00 UTC on 14 September is midnight on 15 September in Warsaw. Reading the UTC date
  // would shade yesterday's square — and a square moved across a streak boundary breaks or
  // invents a run.
  it('files a late-evening UTC instant on the next local day', () => {
    const late = { ...on('2026-09-14'), occurred_at: '2026-09-14T22:00:00Z' } as TimelineEvent;
    const grid = buildActivityGrid([late], TODAY);

    expect(grid.days.find((d) => d.key === '2026-09-15')?.count).toBe(1);
    expect(grid.days.find((d) => d.key === '2026-09-14')?.count).toBe(0);
  });
});

describe('buildActivityGrid counting', () => {
  it('shades a square only for what the candidate did', () => {
    const grid = buildActivityGrid([on('2026-09-10'), on('2026-09-10', 'employer_reply', 'mail_gmail')], TODAY);

    expect(grid.days.find((d) => d.key === '2026-09-10')?.count).toBe(1);
    expect(grid.total).toBe(1);
  });

  // What a square measures is effort; what a day HELD is history. A day whose only event was
  // an employer's reply must still be readable, or the panel would appear to hold nothing.
  it('keeps the non-counting events on the day for the panel to list', () => {
    const grid = buildActivityGrid([on('2026-09-10', 'employer_reply', 'mail_gmail')], TODAY);
    const day = grid.days.find((d) => d.key === '2026-09-10');

    expect(day?.count).toBe(0);
    expect(day?.level).toBe(0);
    expect(day?.events).toHaveLength(1);
  });

  it('orders a day events oldest first', () => {
    const noon = on('2026-09-10');
    const morning = { ...on('2026-09-10'), id: 2, occurred_at: new Date(2026, 8, 10, 8).toISOString() } as TimelineEvent;
    const grid = buildActivityGrid([noon, morning], TODAY);

    expect(grid.days.find((d) => d.key === '2026-09-10')?.events.map((e) => e.id)).toEqual([2, 1]);
  });

  it('ignores an event outside the window', () => {
    const grid = buildActivityGrid([on('2020-01-01'), on('2026-09-10')], TODAY);

    expect(grid.total).toBe(1);
  });
});

describe('buildActivityGrid streaks', () => {
  it('counts consecutive active days ending today', () => {
    const grid = buildActivityGrid(['2026-09-14', '2026-09-15', '2026-09-16'].map((d) => on(d)), TODAY);

    expect(grid.currentStreak).toBe(3);
  });

  // A naive "days ending today" resets at every midnight and tells somebody they lost a run
  // they are still in the middle of. The day is not over.
  it('holds the streak open while today is still running', () => {
    const grid = buildActivityGrid(['2026-09-13', '2026-09-14', '2026-09-15'].map((d) => on(d)), TODAY);

    expect(grid.currentStreak).toBe(3);
  });

  it('ends the streak at a gap', () => {
    const grid = buildActivityGrid(['2026-09-10', '2026-09-11', '2026-09-16'].map((d) => on(d)), TODAY);

    expect(grid.currentStreak).toBe(1);
  });

  it('finds the longest run anywhere in the window', () => {
    const days = ['2026-03-01', '2026-03-02', '2026-03-03', '2026-03-04', '2026-09-16'];
    const grid = buildActivityGrid(days.map((d) => on(d)), TODAY);

    expect(grid.longestStreak).toBe(4);
    expect(grid.currentStreak).toBe(1);
  });

  it('does not let a non-counting event hold a streak open', () => {
    const grid = buildActivityGrid(
      [on('2026-09-14'), on('2026-09-15', 'employer_reply', 'mail_gmail'), on('2026-09-16')],
      TODAY,
    );

    expect(grid.currentStreak).toBe(1);
  });

  it('reports zeroes for a caller who has done nothing', () => {
    const grid = buildActivityGrid([], TODAY);

    expect(grid.total).toBe(0);
    expect(grid.currentStreak).toBe(0);
    expect(grid.longestStreak).toBe(0);
    expect(grid.days.every((d) => d.level === 0)).toBe(true);
  });
});
