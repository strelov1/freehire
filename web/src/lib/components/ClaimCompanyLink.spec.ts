import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// The store only loads in a browser; the components project's stub says otherwise.
vi.mock('$app/env', () => ({ browser: true }));
vi.mock('$app/state', () => ({ page: { data: { locale: 'en' } } }));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));

const { session, getEmployerCompany } = vi.hoisted(() => ({
  session: { userID: null as number | null },
  getEmployerCompany: vi.fn(),
}));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => session.userID !== null }));
vi.mock('#lib/api.js', () => ({ api: { getEmployerCompany } }));

import { markEmployerAccount } from '#lib/employerAccount.svelte.js';
import { resetUserStores } from '#lib/userResource.svelte.js';
import ClaimCompanyLink from './ClaimCompanyLink.svelte';

const NAME = /verify the company/i;

beforeEach(() => {
  resetUserStores();
  session.userID = null;
  getEmployerCompany.mockReset().mockResolvedValue(null);
});

describe('ClaimCompanyLink', () => {
  it('links an unclaimed company to the claim form, named by slug', () => {
    render(ClaimCompanyLink, { slug: 'acme', claimed: false });
    expect(screen.getByRole('link', { name: NAME }).getAttribute('href')).toBe('/my/employer?company=acme');
    expect(getEmployerCompany).not.toHaveBeenCalled();
  });

  it('is hidden once the company is claimed', () => {
    render(ClaimCompanyLink, { slug: 'acme', claimed: true });
    expect(screen.queryByRole('link', { name: NAME })).toBeNull();
  });

  it('stays for a signed-in reader with no employer account', async () => {
    session.userID = 7;
    render(ClaimCompanyLink, { slug: 'acme', claimed: false });
    await tick();
    await tick();
    expect(getEmployerCompany).toHaveBeenCalledOnce();
    expect(screen.getByRole('link', { name: NAME })).toBeTruthy();
  });

  it('is hidden for a reader who already has an employer account', async () => {
    session.userID = 7;
    getEmployerCompany.mockResolvedValue({ company_slug: 'globex' });
    render(ClaimCompanyLink, { slug: 'acme', claimed: false });
    await vi.waitFor(() => expect(screen.queryByRole('link', { name: NAME })).toBeNull());
  });

  it('stays hidden when a lookup that started before a claim answers after it', async () => {
    session.userID = 1;
    let answer!: (v: null) => void;
    getEmployerCompany.mockReturnValue(new Promise((r) => (answer = r)));
    render(ClaimCompanyLink, { slug: 'acme', claimed: false });
    markEmployerAccount();
    answer(null);
    await tick();
    await tick();
    expect(screen.queryByRole('link', { name: NAME })).toBeNull();
  });
});
