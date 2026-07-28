import { describe, it, expect } from 'vitest';
import { splitPresentingCalls, PRESENT_JOBS_TOOL } from './deck';
import type { ToolCall } from './tool-formatters';

/** A settled `present_jobs` call: the model's arguments plus the backend's receipt. */
function presented(
  input: unknown,
  result: unknown,
  extra: Partial<ToolCall> = {},
): ToolCall {
  return { name: PRESENT_JOBS_TOOL, input, result: JSON.stringify(result), ...extra };
}

const twoJobs = {
  heading: 'Top matches',
  jobs: [
    { slug: 'go-dev-acme', note: 'A fintech backend role.', why_fits: ['go', 'postgres'] },
    { slug: 'rust-dev-acme', note: 'Systems work.', concerns: ['on-site once a month'] },
  ],
};

describe('splitPresentingCalls', () => {
  it('takes presenting calls out of the tool-activity list', () => {
    const calls: ToolCall[] = [
      { name: 'search_jobs', input: { query: 'go' }, result: '{}' },
      presented(twoJobs, { presented: ['go-dev-acme', 'rust-dev-acme'], dropped: [] }),
    ];

    const { decks, rest } = splitPresentingCalls(calls);

    expect(rest.map((c) => c.name)).toEqual(['search_jobs']);
    expect(decks).toHaveLength(1);
  });

  it('joins the model rationale onto the slugs the backend accepted', () => {
    const { decks } = splitPresentingCalls([
      presented(twoJobs, { presented: ['go-dev-acme', 'rust-dev-acme'], dropped: [] }),
    ]);

    expect(decks[0]?.status).toBe('ready');
    const deck = decks[0]?.status === 'ready' ? decks[0].deck : null;
    expect(deck?.heading).toBe('Top matches');
    expect(deck?.entries).toEqual([
      {
        slug: 'go-dev-acme',
        note: 'A fintech backend role.',
        whyFits: ['go', 'postgres'],
        concerns: [],
      },
      {
        slug: 'rust-dev-acme',
        note: 'Systems work.',
        whyFits: [],
        concerns: ['on-site once a month'],
      },
    ]);
  });

  it('drops an entry the backend refused', () => {
    const { decks } = splitPresentingCalls([
      presented(
        {
          jobs: [
            { slug: 'go-dev-acme', note: 'real' },
            { slug: 'invented-role', note: 'not real' },
          ],
        },
        {
          presented: ['go-dev-acme'],
          dropped: [{ slug: 'invented-role', reason: 'no vacancy has this public_slug' }],
        },
      ),
    ]);

    const deck = decks[0]?.status === 'ready' ? decks[0].deck : null;
    expect(deck?.entries.map((e) => e.slug)).toEqual(['go-dev-acme']);
  });

  it('renders the deck in the order the backend reported, not the input order', () => {
    const { decks } = splitPresentingCalls([
      presented(
        {
          jobs: [
            { slug: 'c-job', note: '1' },
            { slug: 'a-job', note: '2' },
          ],
        },
        { presented: ['c-job', 'a-job'], dropped: [] },
      ),
    ]);

    const deck = decks[0]?.status === 'ready' ? decks[0].deck : null;
    expect(deck?.entries.map((e) => e.slug)).toEqual(['c-job', 'a-job']);
  });

  it('holds a call still in flight as pending rather than rendering unvalidated slugs', () => {
    const { decks, rest } = splitPresentingCalls([
      { name: PRESENT_JOBS_TOOL, input: twoJobs },
    ]);

    expect(rest).toHaveLength(0);
    // Two placeholders because the call names two vacancies — a count, not a
    // slug, so nothing unvalidated is on screen.
    expect(decks).toEqual([{ status: 'pending', count: 2 }]);
  });

  it('renders nothing for a call the backend rejected', () => {
    const { decks, rest } = splitPresentingCalls([
      presented(twoJobs, { error: 'none of these slugs exist' }, { isError: true }),
    ]);

    // Neither a deck nor a progress chip: the model corrects itself in the same
    // turn, and the user should only ever see the corrected deck.
    expect(decks).toHaveLength(0);
    expect(rest).toHaveLength(0);
  });

  it('renders nothing when the arguments or the receipt are unreadable', () => {
    const cases: ToolCall[] = [
      { name: PRESENT_JOBS_TOOL, input: 'not an object', result: '{"presented":["a"]}' },
      { name: PRESENT_JOBS_TOOL, input: twoJobs, result: 'not json' },
      presented(twoJobs, { presented: [], dropped: [] }),
      presented({ jobs: [] }, { presented: ['go-dev-acme'], dropped: [] }),
    ];

    for (const call of cases) {
      expect(splitPresentingCalls([call]).decks).toHaveLength(0);
    }
  });

  // Svelte throws `each_key_duplicate` on a repeated key in production as well as
  // in dev, and the offending call is already in the transcript — so a duplicate
  // that reached the client would not merely render twice, it would break the
  // session on every reload. The backend refuses duplicates going forward; these
  // guard the transcripts that already contain one.
  it('shows a repeated slug once', () => {
    const { decks } = splitPresentingCalls([
      presented(
        {
          jobs: [
            { slug: 'go-dev-acme', note: 'first' },
            { slug: 'go-dev-acme', note: 'again' },
          ],
        },
        { presented: ['go-dev-acme', 'go-dev-acme'], dropped: [] },
      ),
    ]);

    const deck = decks[0]?.status === 'ready' ? decks[0].deck : null;
    expect(deck?.entries.map((e) => e.slug)).toEqual(['go-dev-acme']);
    // The rationale the model led with, not whatever it repeated.
    expect(deck?.entries[0]?.note).toBe('first');
  });

  it('shows a repeated why_fits or concerns phrase once', () => {
    const { decks } = splitPresentingCalls([
      presented(
        {
          jobs: [
            {
              slug: 'go-dev-acme',
              note: 'n',
              why_fits: ['Go', 'Go', 'Postgres'],
              concerns: ['On-site', 'On-site'],
            },
          ],
        },
        { presented: ['go-dev-acme'], dropped: [] },
      ),
    ]);

    const entry = decks[0]?.status === 'ready' ? decks[0].deck.entries[0] : null;
    expect(entry?.whyFits).toEqual(['Go', 'Postgres']);
    expect(entry?.concerns).toEqual(['On-site']);
  });

  it('renders nothing for an unanswered call on a settled message', () => {
    const inFlight: ToolCall[] = [{ name: PRESENT_JOBS_TOOL, input: twoJobs }];

    // Still running: a placeholder is right.
    expect(splitPresentingCalls(inFlight, true).decks).toEqual([{ status: 'pending', count: 2 }]);
    // Settled with no result — the stream dropped between the call and its
    // result. Skeletons would pulse forever under a finished message.
    expect(splitPresentingCalls(inFlight, false).decks).toHaveLength(0);
  });

  it('keeps several decks in the order they were called', () => {
    const { decks } = splitPresentingCalls([
      presented(
        { heading: 'Top matches', jobs: [{ slug: 'a-job', note: '1' }] },
        { presented: ['a-job'], dropped: [] },
      ),
      { name: 'save_job', input: { slug: 'a-job' }, result: '{}' },
      presented(
        { heading: 'Also worth a look', jobs: [{ slug: 'b-job', note: '2' }] },
        { presented: ['b-job'], dropped: [] },
      ),
    ]);

    const headings = decks.map((d) => (d.status === 'ready' ? d.deck.heading : null));
    expect(headings).toEqual(['Top matches', 'Also worth a look']);
  });
});
