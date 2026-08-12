import { describe, it, expect } from 'vitest';
import { BOARD_COLUMNS, CLOSED_OUTCOMES, boardRefFor, columnOf, matchesQuery } from './board';
import { STAGE_GROUPS } from './generated/contracts';
import { must } from './utils';
import type { MyJob } from './types';

// columnOf reads only `stage` and `applied_at`; a minimal cast fixture suffices.
function job(fields: Partial<MyJob>): MyJob {
  return { stage: null, applied_at: null, saved_at: null, ...fields } as MyJob;
}

// An application with its posting still in the catalogue: the employer and role the
// user reads come from `job`, while `company_slug`/`role_title` carry the copies the
// application recorded when it was made.
function listed(company: string, title: string): MyJob {
  return {
    company_slug: company.toLowerCase().replace(/\s+/g, '-'),
    role_title: title,
    job: { company, title },
  } as MyJob;
}

// An application whose posting cmd/prune removed. There is no `job` to read, so the
// employer survives only as the slug the application copied.
function pruned(companySlug: string, title: string): MyJob {
  return { company_slug: companySlug, role_title: title, job: null } as MyJob;
}

describe('BOARD_COLUMNS', () => {
  it('has no Saved column — only the active application states', () => {
    expect(BOARD_COLUMNS.map((c) => c.id)).toEqual([
      'preparing',
      'applied',
      'interview',
      'offer',
      'closed',
    ]);
  });
});

describe('columnOf', () => {
  it('maps a precise stage to its column', () => {
    expect(columnOf(job({ stage: 'interview' }))).toBe('interview');
    expect(columnOf(job({ stage: 'offer' }))).toBe('offer');
    expect(columnOf(job({ stage: 'screening' }))).toBe('applied');
    expect(columnOf(job({ stage: 'rejected' }))).toBe('closed');
  });

  it('maps an applied-without-stage row to applied', () => {
    expect(columnOf(job({ applied_at: '2026-07-12T00:00:00Z' }))).toBe('applied');
  });

  it('returns null for a saved-only row (not on the board)', () => {
    expect(columnOf(job({ saved_at: '2026-07-12T00:00:00Z' }))).toBeNull();
    expect(columnOf(job({}))).toBeNull();
  });
});

describe('matchesQuery', () => {
  const stripe = listed('Stripe', 'Senior Go Engineer');
  const vercel = listed('Vercel', 'Platform Engineer');

  it('keeps every application when the query is empty or blank', () => {
    expect(matchesQuery(stripe, '')).toBe(true);
    expect(matchesQuery(stripe, '   ')).toBe(true);
  });

  it('matches the employer, ignoring case', () => {
    expect(matchesQuery(stripe, 'stripe')).toBe(true);
    expect(matchesQuery(stripe, 'STRIPE')).toBe(true);
    expect(matchesQuery(vercel, 'stripe')).toBe(false);
  });

  it('matches the role', () => {
    expect(matchesQuery(stripe, 'engineer')).toBe(true);
    expect(matchesQuery(stripe, 'designer')).toBe(false);
  });

  it('matches a partial word, so a search narrows as it is typed', () => {
    expect(matchesQuery(stripe, 'stri')).toBe(true);
    expect(matchesQuery(stripe, 'engin')).toBe(true);
  });

  // The discriminating case for the predicate's shape. A plain substring test over
  // "Stripe Senior Go Engineer" answers false here, because the words are not
  // adjacent in that order — and a candidate typing what they remember does not
  // recall the word order.
  it('requires every word of the query, in any order and across both fields', () => {
    expect(matchesQuery(stripe, 'senior go')).toBe(true);
    expect(matchesQuery(stripe, 'go senior')).toBe(true);
    expect(matchesQuery(stripe, 'stripe engineer')).toBe(true);
    expect(matchesQuery(stripe, 'stripe designer')).toBe(false);
  });

  // The posting is gone, so `job` is null and the employer is known only as the slug
  // the application copied. Reading through `job` would throw here, and skipping the
  // row would hide exactly the applications that are hardest to find another way.
  it('searches a posting-less application by the slug and title it recorded', () => {
    const orphan = pruned('acme-corp', 'Backend Engineer');
    expect(matchesQuery(orphan, 'acme')).toBe(true);
    expect(matchesQuery(orphan, 'backend')).toBe(true);
    expect(matchesQuery(orphan, 'stripe')).toBe(false);
  });

  // A slug is written with dashes and the user types spaces. Whichever way the
  // predicate resolves this, it must not depend on the user guessing the punctuation.
  it('does not make the user type the slug punctuation', () => {
    expect(matchesQuery(pruned('acme-corp', 'Backend Engineer'), 'acme corp')).toBe(true);
  });
});

describe('boardRefFor', () => {
  it('addresses a posting-backed application by its posting slug', () => {
    expect(boardRefFor({ job_slug: 'senior-go-engineer-derq-1a2b', application_id: 42 })).toBe(
      'senior-go-engineer-derq-1a2b',
    );
  });

  // The bare id matches no row the listing mints, so the drawer would never open.
  it('prefixes an application whose posting was pruned, never serving the bare id', () => {
    expect(boardRefFor({ application_id: 42 })).toBe('a42');
  });

  it('addresses nothing when the event names no application', () => {
    expect(boardRefFor({})).toBeNull();
  });
});

// The outcomes offered when a card is dropped into Closed must BE the closed group, not a
// second list that happens to agree with it today. A hand-written copy is exactly how a new
// settled stage becomes unreachable from the board while every type check still passes: the
// generated-stage check binds Stage to STAGE_GROUPS, and says nothing about a literal array.
describe('CLOSED_OUTCOMES', () => {
  it('is the generated closed group', () => {
    const closed = must(STAGE_GROUPS.find((g) => g.id === 'closed'));
    expect([...CLOSED_OUTCOMES]).toEqual([...closed.stages]);
  });
});
