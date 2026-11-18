import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import EmployerClaimsQueue from './EmployerClaimsQueue.svelte';

const { listPendingEmployerClaims, approveEmployerClaim, rejectEmployerClaim } = vi.hoisted(() => ({
  listPendingEmployerClaims: vi.fn(),
  approveEmployerClaim: vi.fn(),
  rejectEmployerClaim: vi.fn(),
}));

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));
vi.mock('#lib/api.js', () => ({
  api: { listPendingEmployerClaims, approveEmployerClaim, rejectEmployerClaim },
  ApiError: class extends Error {},
}));

const claim = {
  user_id: 1,
  company_slug: 'acme',
  company_name: 'Acme',
  work_email: 'hr@other.test',
  status: 'pending',
  email_confirmed: true,
  member: false,
  created_at: '2026-11-01T00:00:00Z',
  website: 'https://acme.test',
  review_reason: 'domain_mismatch',
};

describe('EmployerClaimsQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listPendingEmployerClaims.mockResolvedValue([claim]);
  });

  it.each([
    ['unknown_website', '', 'Company website is unknown.'],
    ['unknown_website', 'not a website', 'Company website is unknown.'],
    ['domain_mismatch', 'https://acme.test', 'Work email domain does not match the company website.'],
    ['academic_domain', 'https://example.edu', 'Academic email domains require manual review.'],
    ['manual_review', 'https://other.test', 'Domains currently match; this claim still needs manual review.'],
  ])('shows the current check for %s', async (review_reason, website, reason) => {
    listPendingEmployerClaims.mockResolvedValue([{ ...claim, website, review_reason }]);
    render(EmployerClaimsQueue);
    expect(await screen.findByText(reason)).toBeTruthy();
    expect(screen.getByText(`Known website: ${website || 'Not recorded'}`)).toBeTruthy();
  });

  it('keeps approval and rejection working with review details', async () => {
    listPendingEmployerClaims.mockResolvedValue([claim, { ...claim, user_id: 2, company_name: 'Other' }]);
    render(EmployerClaimsQueue);
    await screen.findAllByText(/Work email domain does not match/);
    const [firstClaim] = screen.getAllByRole('listitem');
    if (!firstClaim) throw new Error('Expected a pending claim');
    await fireEvent.click(within(firstClaim).getByRole('button', { name: 'Approve' }));
    await vi.waitFor(() => expect(approveEmployerClaim).toHaveBeenCalledWith(1));
    await vi.waitFor(() => expect(screen.queryByText('Acme')).toBeNull());
    await fireEvent.click(screen.getByRole('button', { name: /^Reject$/ }));
    await fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Reject' }));
    await vi.waitFor(() => expect(rejectEmployerClaim).toHaveBeenCalledWith(2));
    expect(await screen.findByText('No employer claims awaiting review.')).toBeTruthy();
  });
});
