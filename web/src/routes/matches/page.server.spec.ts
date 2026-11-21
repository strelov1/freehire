import { beforeEach, describe, expect, it, vi } from 'vitest';

const api = { plans: vi.fn() };
vi.mock('#lib/server/api.js', () => ({ serverApi: () => api }));
vi.mock('$app/env/public', () => ({ PUBLIC_MATCHES: '1' }));

const { load } = await import('./+page.server.js');
type LoadEvent = Parameters<typeof load>[0];

const run = (tier?: string) =>
  load({
    parent: async () => ({ user: { tier } }),
    url: new URL('https://x/matches'),
    fetch,
  } as unknown as LoadEvent);

describe('matches load: paid gate', () => {
  beforeEach(() => vi.clearAllMocks());

  it('free account gets the upgrade screen with a price', async () => {
    const pro = { id: 'price_pro', tier: 'pro', interval: 'month', amount_cents: 500, currency: 'usd' };
    api.plans.mockResolvedValue({ prices: [{ ...pro, tier: 'ultra', id: 'u' }, pro] });
    expect(await run('free')).toEqual({ paid: false, price: pro });
  });

  it('a missing tier counts as free; a failed price load leaves price null', async () => {
    api.plans.mockRejectedValue(new Error('down'));
    expect(await run(undefined)).toEqual({ paid: false, price: null });
  });

  it.each(['pro', 'ultra'])('%s account gets the feed, no plans call', async (tier) => {
    expect(await run(tier)).toEqual({ paid: true, price: null });
    expect(api.plans).not.toHaveBeenCalled();
  });
});
