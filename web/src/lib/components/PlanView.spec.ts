import { render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Allowance, PlanState } from '#lib/types.js';
import PlanView from './PlanView.svelte';
import { page } from '$app/state';
import { track } from '#lib/analytics.js';
import { beginCheckoutTracking } from '#lib/purchaseTracking.js';
import { saveTailorContext } from '#lib/tailorCheckout.js';
import { tick } from 'svelte';

const account = await vi.hoisted(async () => {
  const { SvelteMap } = await import('svelte/reactivity');
  return new SvelteMap([['id', 7]]);
});

vi.mock('$app/paths', () => ({
  resolve: (p: string, params?: Record<string, string>) => p.replace('[slug]', params?.slug ?? ''),
  base: '',
  assets: '',
}));
vi.mock('$app/navigation', () => ({
  replaceState: vi.fn(),
  goto: vi.fn(),
  invalidateAll: vi.fn().mockResolvedValue(undefined),
}));
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/my/plan'), state: {} } }));
vi.mock('#lib/analytics.js', () => ({ track: vi.fn() }));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => true,
  currentUser: () => ({ id: account.get('id'), beta_tester: false }),
}));
const { myPlan, myPlanHistory, getApiPlan, billingManageUrl, billingAttempts, billingSubscription, plans } = vi.hoisted(
  () => ({
    myPlan: vi.fn(),
    myPlanHistory: vi.fn(),
    getApiPlan: vi.fn(),
    billingManageUrl: vi.fn(),
    billingAttempts: vi.fn(),
    billingSubscription: vi.fn().mockRejectedValue(new Error('none')),
    plans: vi.fn().mockResolvedValue({ prices: [] }),
  }),
);
vi.mock('#lib/api.js', () => ({
  api: { myPlan, myPlanHistory, getApiPlan, billingManageUrl, billingAttempts, billingSubscription, plans },
}));

beforeEach(() => {
  account.set('id', 7);
  billingAttempts.mockResolvedValue([]);
  page.url = new URL('http://localhost/my/plan');
});

const allowance = (feature: string, over: Partial<Allowance> = {}): Allowance => ({
  feature,
  used: 0,
  unlimited: false,
  enforced: true,
  resets_at: '2026-10-26T00:00:00Z',
  ...over,
});

function freePlan(allowances: Allowance[]): PlanState {
  return { plan: 'free', resets_at: '2026-10-26T00:00:00Z', allowances };
}

describe('PlanView allowances', () => {
  beforeEach(() => {
    myPlanHistory.mockResolvedValue([]);
    getApiPlan.mockResolvedValue({ tier: '' });
  });

  it('labels every metered feature instead of printing its API key', async () => {
    myPlan.mockResolvedValue(freePlan([allowance('cover-letter'), allowance('auto-apply')]));
    render(PlanView);
    expect(await screen.findByText('Cover letters')).toBeTruthy();
    expect(screen.getByText('Automatic applications')).toBeTruthy();
    expect(screen.queryByText('cover-letter')).toBeNull();
    expect(screen.queryByText('auto-apply')).toBeNull();
  });

  it('humanises a feature key the strings do not know yet', async () => {
    myPlan.mockResolvedValue(freePlan([allowance('interview-prep', { limit: 2 })]));
    render(PlanView);
    expect(await screen.findByText('Interview prep')).toBeTruthy();
  });

  it('says a zero allowance is not on the plan rather than rendering a dangling "0 /"', async () => {
    myPlan.mockResolvedValue(freePlan([allowance('tailor')]));
    render(PlanView);
    expect(await screen.findByText('Not on Free')).toBeTruthy();
    expect(screen.queryByText(/0 \//)).toBeNull();
  });

  it('shows used against limit when the plan includes the feature', async () => {
    myPlan.mockResolvedValue(freePlan([allowance('match', { used: 1, limit: 3 })]));
    render(PlanView);
    expect(await screen.findByText('1 / 3')).toBeTruthy();
  });
});

describe('PlanView API plan', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    myPlanHistory.mockResolvedValue([]);
    myPlan.mockResolvedValue(freePlan([]));
    billingManageUrl.mockResolvedValue({ url: 'https://billing.example/portal' });
  });

  it('shows a bought API plan and says it does not need Pro', async () => {
    getApiPlan.mockResolvedValue({ tier: 'api', until: '2026-12-09T12:00:00Z' });
    render(PlanView);
    expect(await screen.findByText('API plan')).toBeTruthy();
    expect(screen.getByText('Pro is for AI job-search features. Your API plan does not need it.')).toBeTruthy();
    expect((await screen.findByRole('link', { name: 'Manage or cancel' })).getAttribute('href')).toBe(
      'https://billing.example/portal',
    );
  });

  it('shows no API strip and no portal link to an account without one', async () => {
    getApiPlan.mockResolvedValue({ tier: '' });
    render(PlanView);
    expect(await screen.findByText('Same features, daily limits')).toBeTruthy();
    expect(screen.queryByText('API plan')).toBeNull();
    expect(billingManageUrl).not.toHaveBeenCalled();
  });

  it('says it could not check the API plan instead of reading as none', async () => {
    getApiPlan.mockRejectedValue(new Error('network'));
    render(PlanView);
    expect(await screen.findByText("Couldn't check your API plan. Reload the page to try again.")).toBeTruthy();
  });
});

describe('PlanView purchase confirmation', () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.mocked(track).mockClear();
    page.url = new URL('http://localhost/my/plan?checkout=success');
    myPlan.mockResolvedValue(freePlan([]));
    myPlanHistory.mockResolvedValue([]);
    getApiPlan.mockResolvedValue({ tier: '' });
  });

  it('reports the confirmed charge even before the entitlement catches up', async () => {
    beginCheckoutTracking(7, [], sessionStorage, Date.now() - 2000);
    billingAttempts.mockResolvedValue([
      { id: 'ch_confirmed', date: new Date().toISOString(), amount_cents: 999, currency: 'usd', status: 'succeeded' },
    ]);
    render(PlanView);
    await vi.waitFor(() =>
      expect(track).toHaveBeenCalledWith('subscribe', { transaction_id: 'ch_confirmed', value: 9.99, currency: 'USD' }),
    );
  });

  it('does not treat the return marker as a purchase', async () => {
    billingAttempts.mockResolvedValue([]);
    render(PlanView);
    await vi.waitFor(() => expect(billingAttempts).toHaveBeenCalled());
    expect(track).not.toHaveBeenCalledWith('subscribe', expect.anything());
    page.url = new URL('http://localhost/my/plan');
  });
});

describe('PlanView contextual tailoring continuation', () => {
  beforeEach(() => {
    sessionStorage.clear();
    myPlanHistory.mockResolvedValue([]);
    getApiPlan.mockResolvedValue({ tier: '' });
    billingAttempts.mockResolvedValue([]);
    saveTailorContext(7, { slug: 'senior-go-acme-1', baseCvId: 'base-b' }, sessionStorage);
  });

  it('offers to continue only after a fresh read shows a paid plan, though the page itself loaded Free', async () => {
    page.url = new URL('http://localhost/my/plan?checkout=success');
    myPlan.mockResolvedValueOnce(freePlan([])).mockResolvedValue({ ...freePlan([]), plan: 'pro' });
    render(PlanView);
    expect(await screen.findByRole('button', { name: 'Continue tailoring' }, { timeout: 5000 })).toBeTruthy();
    expect(screen.getByText('Pro')).toBeTruthy();
  });

  it('does not treat an API-only plan as Pro', async () => {
    page.url = new URL('http://localhost/my/plan');
    myPlan.mockResolvedValue(freePlan([]));
    getApiPlan.mockResolvedValue({ tier: 'api' });
    render(PlanView);
    expect(await screen.findByText('Your tailoring task is saved')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
  });

  it('shows nothing extra for a user with no saved task', async () => {
    sessionStorage.clear();
    page.url = new URL('http://localhost/my/plan');
    myPlan.mockResolvedValue(freePlan([]));
    render(PlanView);
    await screen.findByText('Your plan');
    expect(screen.queryByText('Your tailoring task is saved')).toBeNull();
  });

  it('clears the previous account plan before the new account read finishes', async () => {
    const next = Promise.withResolvers<PlanState>();
    myPlan.mockResolvedValueOnce({ ...freePlan([]), plan: 'pro' }).mockReturnValue(next.promise);
    render(PlanView);
    await screen.findByRole('button', { name: 'Continue tailoring' });
    saveTailorContext(8, { slug: 'other-role-2' }, sessionStorage);
    account.set('id', 8);
    await tick();
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
    next.resolve(freePlan([]));
    await screen.findByText('Your tailoring task is saved');
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
  });

  it('ignores the previous account late plan response', async () => {
    const previous = Promise.withResolvers<PlanState>();
    myPlan.mockReturnValueOnce(previous.promise).mockResolvedValue(freePlan([]));
    render(PlanView);
    await tick();
    saveTailorContext(8, { slug: 'other-role-2' }, sessionStorage);
    account.set('id', 8);
    await screen.findByText('Your tailoring task is saved');
    previous.resolve({ ...freePlan([]), plan: 'pro' });
    await previous.promise;
    await tick();
    expect(screen.queryByRole('button', { name: 'Continue tailoring' })).toBeNull();
  });

  it('does not replace newly verified Pro with an older Free read waiting for history', async () => {
    const history = Promise.withResolvers<[]>();
    myPlanHistory.mockReturnValueOnce(history.promise);
    page.url = new URL('http://localhost/my/plan?checkout=success');
    myPlan
      .mockResolvedValueOnce(freePlan([]))
      .mockResolvedValueOnce(freePlan([]))
      .mockResolvedValue({ ...freePlan([]), plan: 'pro' });
    render(PlanView);
    await screen.findByRole('button', { name: 'Continue tailoring' }, { timeout: 5000 });
    history.resolve([]);
    await history.promise;
    await tick();
    await waitFor(() => expect(screen.queryByText('Pro')).toBeTruthy());
  });
});
