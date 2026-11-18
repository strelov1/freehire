import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/') } }));

import { onboardingGate, routeAccount } from './onboardingGate.svelte';

const deps = () => ({ complete: vi.fn().mockResolvedValue(undefined), go: vi.fn() });
const MINUTE = 60_000;

beforeEach(() => {
  sessionStorage.clear();
  onboardingGate.reset();
});

describe('routing a signed-in account', () => {
  it('sends an account that has not been onboarded to onboarding once, carrying where it was', () => {
    const d = deps();
    routeAccount('/jobs/acme', '?tab=discussion', true, d);
    expect(d.go).toHaveBeenCalledExactlyOnceWith('/onboarding?returnTo=%2Fjobs%2Facme%3Ftab%3Ddiscussion');
    expect(d.complete).not.toHaveBeenCalled();
  });

  it('leaves an onboarded account alone', () => {
    const d = deps();
    routeAccount('/jobs/acme', '', false, d);
    expect(d.go).not.toHaveBeenCalled();
  });

  it('leaves /onboarding and /signin alone, and does not spend a guest skip there', () => {
    onboardingGate.skipAsGuest();
    const d = deps();
    routeAccount('/signin', '', true, d);
    routeAccount('/onboarding', '', true, d);
    expect(d.go).not.toHaveBeenCalled();
    expect(onboardingGate.takeGuestSkip()).toBe(true);
  });
});

describe('a guest who skipped onboarding to authenticate', () => {
  it('is not bounced back after a same-tab password sign-up; onboarding is marked done once', () => {
    onboardingGate.skipAsGuest();
    const d = deps();
    routeAccount('/signin', '', true, d); // the credential page passes through first
    routeAccount('/jobs/acme', '', true, d);
    routeAccount('/jobs/acme', '?x=1', true, d);
    expect(d.go).not.toHaveBeenCalled();
    expect(d.complete).toHaveBeenCalledTimes(1);
    expect(sessionStorage.length).toBe(0);
  });

  it('is not bounced back after an OAuth round trip, which loses everything but storage', () => {
    onboardingGate.skipAsGuest();
    // A full page load: the module state is fresh, the tab's storage is not.
    const d = deps();
    routeAccount('/jobs/acme', '', true, d);
    expect(d.go).not.toHaveBeenCalled();
    expect(d.complete).toHaveBeenCalledTimes(1);
  });

  it('spends the skip on an account that was already onboarded, so the next account inherits nothing', () => {
    onboardingGate.skipAsGuest();
    const existing = deps();
    routeAccount('/jobs/acme', '', false, existing); // an existing user logged in through the skip
    expect(existing.complete).not.toHaveBeenCalled();
    onboardingGate.reset(); // signed out
    const next = deps();
    routeAccount('/jobs/acme', '', true, next); // a brand-new account on the same tab
    expect(next.go).toHaveBeenCalledTimes(1);
    expect(next.complete).not.toHaveBeenCalled();
  });

  it('drops an abandoned skip when the session ends', () => {
    onboardingGate.skipAsGuest();
    onboardingGate.reset();
    expect(onboardingGate.takeGuestSkip()).toBe(false);
  });

  it('does not excuse an account that appears long after the skip', () => {
    const then = Date.now();
    onboardingGate.skipAsGuest(then);
    expect(onboardingGate.takeGuestSkip(then + 31 * MINUTE)).toBe(false);
    onboardingGate.skipAsGuest(then);
    expect(onboardingGate.takeGuestSkip(then + 29 * MINUTE)).toBe(true);
  });

  it('is not taken when nobody skipped, or twice', () => {
    expect(onboardingGate.takeGuestSkip()).toBe(false);
    onboardingGate.skipAsGuest();
    expect(onboardingGate.takeGuestSkip()).toBe(true);
    expect(onboardingGate.takeGuestSkip()).toBe(false);
  });

  it('survives a failing completion call', () => {
    onboardingGate.skipAsGuest();
    const d = deps();
    d.complete.mockRejectedValue(new Error('offline'));
    expect(() => routeAccount('/', '', true, d)).not.toThrow();
  });
});
