import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Allowance, PlanState } from '#lib/types.js';
import PlanView from './PlanView.svelte';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ replaceState: vi.fn() }));
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/my/plan'), state: {} } }));
vi.mock('#lib/analytics.js', () => ({ track: vi.fn() }));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => true,
  currentUser: () => ({ beta_tester: false }),
}));
const { myPlan, myPlanHistory, getApiPlan, billingManageUrl } = vi.hoisted(() => ({
  myPlan: vi.fn(),
  myPlanHistory: vi.fn(),
  getApiPlan: vi.fn(),
  billingManageUrl: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({ api: { myPlan, myPlanHistory, getApiPlan, billingManageUrl } }));

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
