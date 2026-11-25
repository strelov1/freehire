import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getEmployerCompany, getCompany, claimCompany, search } = vi.hoisted(() => ({
  getEmployerCompany: vi.fn(),
  getCompany: vi.fn(),
  claimCompany: vi.fn(),
  search: { value: '' },
}));

vi.mock('$app/state', () => ({
  page: {
    data: { locale: 'en' },
    get url() {
      return new URL(`http://localhost/my/employer${search.value}`);
    },
  },
}));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/api.js', () => ({
  api: { getEmployerCompany, getCompany, claimCompany },
  ApiError: class extends Error {},
}));
vi.mock('./EmployerDashboard.svelte', () => ({ default: () => {} }));

import { hasEmployerAccount } from '#lib/employerAccount.svelte.js';
import EmployerView from './EmployerView.svelte';

const pending = {
  user_id: 1,
  company_slug: 'acme',
  company_name: 'Acme Inc.',
  work_email: 'hr@acme.test',
  status: 'pending',
  email_confirmed: false,
  member: false,
  created_at: '2026-11-01T00:00:00Z',
};

beforeEach(() => {
  vi.clearAllMocks();
  search.value = '';
  getEmployerCompany.mockResolvedValue(null);
  getCompany.mockResolvedValue({ company: { name: 'Acme Inc.' } });
  claimCompany.mockResolvedValue(pending);
});

async function submitWith(email: string) {
  await fireEvent.input(await screen.findByLabelText(/work email/i), { target: { value: email } });
  await fireEvent.click(screen.getByRole('button', { name: 'Claim company' }));
}

describe('EmployerView claim form', () => {
  it('with ?company=<slug> shows the catalogue name and claims by slug', async () => {
    search.value = '?company=acme';
    render(EmployerView, { section: 'profile' });
    expect(await screen.findByText('Acme Inc.')).toBeTruthy();
    expect(screen.queryByPlaceholderText('Acme')).toBeNull();
    expect(getCompany).toHaveBeenCalledWith('acme', 1, 0);

    await submitWith('hr@acme.test');
    expect(claimCompany).toHaveBeenCalledWith({ slug: 'acme' }, 'hr@acme.test');
    // The next company page must not offer a second claim.
    expect(hasEmployerAccount()).toBe(true);
  });

  it('does not submit while the slug lookup is pending', async () => {
    search.value = '?company=acme';
    let resolve!: (v: { company: { name: string } }) => void;
    getCompany.mockReturnValue(new Promise((r) => (resolve = r)));
    render(EmployerView, { section: 'profile' });
    await submitWith('hr@acme.test');
    expect(claimCompany).not.toHaveBeenCalled();

    resolve({ company: { name: 'Acme Inc.' } });
    expect(await screen.findByText('Acme Inc.')).toBeTruthy();
    const button = screen.getByRole('button', { name: 'Claim company' }) as HTMLButtonElement;
    await waitFor(() => expect(button.disabled).toBe(false));
    await fireEvent.click(button);
    expect(claimCompany).toHaveBeenCalledWith({ slug: 'acme' }, 'hr@acme.test');
  });

  it('falls back to the typed name when the slug does not resolve', async () => {
    search.value = '?company=ghost';
    getCompany.mockRejectedValue(new Error('404'));
    render(EmployerView, { section: 'profile' });
    await fireEvent.input(await screen.findByPlaceholderText('Acme'), { target: { value: 'Ghost Co' } });
    await submitWith('hr@ghost.test');
    expect(claimCompany).toHaveBeenCalledWith({ name: 'Ghost Co' }, 'hr@ghost.test');
  });
});
