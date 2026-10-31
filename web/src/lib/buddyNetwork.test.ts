import { describe, expect, it } from 'vitest';
import { buddiesByCompany } from './buddyNetwork';
import type { BuddySimilar } from './types';

const summary = { category: 'backend', seniority: 'mid', country: 'de' };
function member(handle: string, companies: string[]): BuddySimilar {
  return { handle, card_summary: summary, score: 1, shared_companies: companies.map((n) => ({ slug: n.toLowerCase(), name: n })) };
}

describe('buddiesByCompany', () => {
  it('lists each shared company with the members who applied there, busiest first', () => {
    const groups = buddiesByCompany([
      member('a-a-1', ['GitLab', 'Stripe']),
      member('b-b-2', ['GitLab']),
      member('c-c-3', ['Revolut', 'GitLab']),
      member('d-d-4', ['Stripe']),
    ]);
    expect(groups.map((g) => [g.company.name, g.members.map((m) => m.handle)])).toEqual([
      ['GitLab', ['a-a-1', 'b-b-2', 'c-c-3']],
      ['Stripe', ['a-a-1', 'd-d-4']],
      ['Revolut', ['c-c-3']],
    ]);
  });

  it('breaks a tie by company name, so the order does not flicker between loads', () => {
    const groups = buddiesByCompany([member('a-a-1', ['Zeta', 'Alpha'])]);
    expect(groups.map((g) => g.company.name)).toEqual(['Alpha', 'Zeta']);
  });

  it('is empty when nobody shares a company', () => {
    expect(buddiesByCompany([member('a-a-1', [])])).toEqual([]);
  });
});
