import { beforeEach, describe, expect, it, vi } from 'vitest';

const pageState = vi.hoisted(() => ({ url: new URL('http://localhost/jobs?sort=newest') }));
const nav = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock('$app/state', () => ({ page: pageState }));
vi.mock('$app/navigation', () => nav);
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));

import { promptLogin, promptSignIn, promptSignInFor } from './signin';

const target = () => new URL(String(nav.goto.mock.calls.at(-1)?.[0]), 'http://localhost');

beforeEach(() => {
  nav.goto.mockClear();
  pageState.url = new URL('http://localhost/jobs?sort=newest');
});

describe('the guest action gate', () => {
  it('sends a guest to onboarding first, carrying the page and its filters', () => {
    promptSignIn();
    expect(target().pathname).toBe('/onboarding');
    expect(target().searchParams.get('returnTo')).toBe('/jobs?sort=newest');
  });

  it('carries an explicit address the page URL does not show yet', () => {
    promptSignInFor('/jobs/acme-engineer?tab=discussion');
    expect(target().pathname).toBe('/onboarding');
    expect(target().searchParams.get('returnTo')).toBe('/jobs/acme-engineer?tab=discussion');
  });

  it('goes straight to the sign-in form from onboarding rather than looping into itself', () => {
    pageState.url = new URL('http://localhost/onboarding?returnTo=%2Fjobs');
    promptSignIn();
    expect(target().pathname).toBe('/signin');
    expect(target().searchParams.get('mode')).toBe('login');
  });

  it('goes straight to the sign-in form from the credential page', () => {
    pageState.url = new URL('http://localhost/signin');
    promptSignInFor('/jobs');
    expect(target().pathname).toBe('/signin');
  });
});

describe('the explicit sign-in', () => {
  it('opens the sign-in form and returns to the page, never through onboarding', () => {
    promptLogin();
    expect(target().pathname).toBe('/signin');
    expect(target().searchParams.get('mode')).toBe('login');
    expect(target().searchParams.get('returnTo')).toBe('/jobs?sort=newest');
  });
});
