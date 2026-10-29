import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import RateLimitLadder from './RateLimitLadder.svelte';

describe('RateLimitLadder', () => {
  it('lists every rung with its budget, and says when a day has no cap', () => {
    render(RateLimitLadder, {
      tiers: [
        { tier: 'anonymous', per_minute: 120, per_day: 2000 },
        { tier: 'pro', per_minute: 800, per_day: 30000 },
        { tier: 'api_data', per_minute: 6000, per_day: 0 },
      ],
    });
    const rows = screen
      .getAllByRole('row')
      .slice(1)
      .map((r) => Array.from(r.querySelectorAll('td'), (td) => td.textContent?.trim()));
    expect(rows).toEqual([
      ['No key', '120', '2,000'],
      ['Pro', '800', '30,000'],
      ['Data', '6,000', 'No cap'],
    ]);
  });
});
