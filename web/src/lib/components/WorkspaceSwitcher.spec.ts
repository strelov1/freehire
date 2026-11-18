import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import WorkspaceSwitcher from './WorkspaceSwitcher.svelte';

const state = vi.hoisted(() => ({
  page: { url: new URL('http://localhost/my/profile') },
  user: { beta_tester: false, role: 'user' },
}));
vi.mock('$app/state', () => ({ page: state.page }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => '/' + path.replace(/^\//, '') }));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => state.user }));
vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));

afterEach(() => {
  cleanup();
  state.page.url = new URL('http://localhost/my/profile');
  state.user.beta_tester = false;
});

const mount = () => {
  const onselect = vi.fn();
  render(WorkspaceSwitcher, { rowClass: 'test-row', onselect });
  return onselect;
};

describe('account menu accordions', () => {
  it('opens the current group and marks only the most specific page', () => {
    state.page.url = new URL('http://localhost/my/referrals/incoming');
    mount();
    expect(screen.getByRole('menuitem', { name: 'Referrer' }).getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByRole('menuitem', { name: 'Incoming requests' }).getAttribute('aria-current')).toBe('page');
    expect(screen.queryByRole('menuitem', { name: 'Profile' })).toBeNull();
  });

  it('expands a group without navigating or closing the menu, then closes on a destination', async () => {
    const onselect = mount();
    const search = screen.getByRole('menuitem', { name: 'My job search' });
    expect(search.getAttribute('aria-expanded')).toBe('true');
    await fireEvent.click(screen.getByRole('menuitem', { name: 'Employer' }));
    expect(onselect).not.toHaveBeenCalled();
    expect(search.getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByRole('menuitem', { name: 'Tracking' })).toBeNull();
    const vacancies = screen.getByRole('menuitem', { name: 'Vacancies' });
    expect(vacancies.getAttribute('href')).toBe('/my/employer/vacancies');
    await fireEvent.click(vacancies);
    expect(onselect).toHaveBeenCalledOnce();
  });

  it('allows collapsing the active group and keeps buddies a direct link', async () => {
    mount();
    const search = screen.getByRole('menuitem', { name: 'My job search' });
    await fireEvent.click(search);
    expect(search.getAttribute('aria-expanded')).toBe('false');
    const buddies = screen.getByRole('menuitem', { name: 'Interview buddies' });
    expect(buddies.getAttribute('href')).toBe('/buddies');
    expect(buddies.hasAttribute('aria-expanded')).toBe(false);
  });

  it('opens shared settings on a settings page without repeating them in job search', async () => {
    state.page.url = new URL('http://localhost/my/account');
    mount();
    expect(screen.getByRole('menuitem', { name: 'Settings' }).getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByRole('menuitem', { name: 'Account' }).getAttribute('aria-current')).toBe('page');
    await fireEvent.click(screen.getByRole('menuitem', { name: 'My job search' }));
    expect(screen.queryByRole('menuitem', { name: 'Account' })).toBeNull();
    expect(screen.getByRole('menuitem', { name: 'Profile' }).getAttribute('href')).toBe('/my/profile');
  });

  it('starts collapsed outside the account area', () => {
    state.page.url = new URL('http://localhost/jobs');
    mount();
    expect(screen.getByRole('menuitem', { name: 'My job search' }).getAttribute('aria-expanded')).toBe('false');
    expect(screen.getByRole('menuitem', { name: 'Settings' }).getAttribute('aria-expanded')).toBe('false');
  });

  it('keeps mentor navigation behind the beta gate', async () => {
    mount();
    expect(screen.queryByRole('menuitem', { name: 'Mentor' })).toBeNull();
    expect(screen.queryByRole('menuitem', { name: 'Mentorship' })).toBeNull();
    cleanup();
    state.user.beta_tester = true;
    mount();
    await fireEvent.click(screen.getByRole('menuitem', { name: 'Mentor' }));
    expect(screen.getByRole('menuitem', { name: 'Schedule' }).getAttribute('href')).toBe('/my/mentorship/schedule');
  });
});
