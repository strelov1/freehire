import { describe, expect, it, vi } from 'vitest';

// Captures the options the module hands to posthog.init, so before_send is exercised as wired.
const init = vi.fn();
vi.mock('posthog-js', () => ({ default: { init } }));

describe('PostHog before_send', () => {
  it('strips booking-link tokens from event and person URL properties', async () => {
    Object.assign(globalThis, {
      window: globalThis,
      location: { hostname: 'freehire.me' },
      document: { createElement: () => ({}), head: { appendChild: () => {} } },
    });
    const { initTrackers } = await import('./analytics');
    initTrackers({ key: 'phc_test', apiHost: '/ingest' });
    await vi.waitFor(() => expect(init).toHaveBeenCalled());
    const beforeSend = init.mock.lastCall?.[1].before_send;

    const url = 'https://freehire.me/booking/SECRETtoken123?x=1';
    const out = beforeSend({
      event: '$pageview',
      properties: { $current_url: url, plan: 'pro', $set_once: { $initial_current_url: url } },
      $set: { $current_url: url },
      $set_once: { $initial_current_url: url, $initial_referrer: url },
    });

    expect(JSON.stringify(out)).not.toContain('SECRETtoken123');
    expect(out.properties.$current_url).toBe('https://freehire.me/booking/:token?x=1');
    expect(out.properties.$set_once.$initial_current_url).toBe('https://freehire.me/booking/:token?x=1');
    expect(out.$set_once.$initial_referrer).toBe('https://freehire.me/booking/:token?x=1');
    expect(out.properties.plan).toBe('pro');
  });
});
