import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { EmployerTeam as Team } from '#lib/types.js';
import EmployerTeam from './EmployerTeam.svelte';

const { getEmployerTeam, inviteEmployerTeamMember, withdrawEmployerTeamInvitation, removeEmployerTeamMember } =
  vi.hoisted(() => ({
    getEmployerTeam: vi.fn(),
    inviteEmployerTeamMember: vi.fn(),
    withdrawEmployerTeamInvitation: vi.fn(),
    removeEmployerTeamMember: vi.fn(),
  }));

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => ({ id: 2 }), isAuthenticated: () => true }));
vi.mock('#lib/api.js', () => ({
  api: { getEmployerTeam, inviteEmployerTeamMember, withdrawEmployerTeamInvitation, removeEmployerTeamMember },
  ApiError: class extends Error {},
}));

const team: Team = {
  members: [
    { user_id: 1, work_email: 'hr@acme.test', joined_at: '2026-10-01T00:00:00Z', holder: true },
    { user_id: 2, work_email: 'me@acme.test', joined_at: '2026-11-01T00:00:00Z', holder: false },
    { user_id: 3, work_email: 'dev@acme.test', joined_at: '2026-11-02T00:00:00Z', holder: false },
  ],
  invitations: [
    { id: 9, email: 'new@acme.test', created_at: '2026-11-10T00:00:00Z', expires_at: '2026-11-17T00:00:00Z' },
  ],
};

describe('EmployerTeam', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getEmployerTeam.mockResolvedValue(team);
  });

  it('lists the team: the holder cannot be removed, the caller may leave, others may be removed', async () => {
    render(EmployerTeam, { onLeft: vi.fn() });
    expect(await screen.findByText('hr@acme.test')).toBeTruthy();
    expect(screen.getByText('Account holder')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Leave the team' })).toBeTruthy();
    expect(screen.getAllByRole('button', { name: 'Remove' })).toHaveLength(1);
    expect(screen.getByText('new@acme.test')).toBeTruthy();
  });

  it('invites a colleague and reloads the team', async () => {
    inviteEmployerTeamMember.mockResolvedValue(team.invitations[0]);
    render(EmployerTeam, { onLeft: vi.fn() });
    await fireEvent.input(await screen.findByLabelText(/Invite a colleague/), { target: { value: ' new@acme.test ' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Send invitation' }));
    expect(inviteEmployerTeamMember).toHaveBeenCalledWith('new@acme.test');
    expect(await screen.findByText('Invitation sent to new@acme.test.')).toBeTruthy();
    expect(getEmployerTeam).toHaveBeenCalledTimes(2);
  });

  it('withdraws an invitation', async () => {
    withdrawEmployerTeamInvitation.mockResolvedValue(undefined);
    render(EmployerTeam, { onLeft: vi.fn() });
    await fireEvent.click(await screen.findByRole('button', { name: 'Withdraw' }));
    expect(withdrawEmployerTeamInvitation).toHaveBeenCalledWith(9);
  });

  it('removes the colleague it was asked to, after confirming', async () => {
    removeEmployerTeamMember.mockResolvedValue(undefined);
    const onLeft = vi.fn();
    render(EmployerTeam, { onLeft });
    await fireEvent.click(await screen.findByRole('button', { name: 'Remove' }));
    await fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Remove' }));
    await vi.waitFor(() => expect(removeEmployerTeamMember).toHaveBeenCalledWith(3));
    await vi.waitFor(() => expect(getEmployerTeam).toHaveBeenCalledTimes(2));
    expect(onLeft).not.toHaveBeenCalled();
  });

  it('leaves the team, and reports leaving only once it succeeded', async () => {
    const onLeft = vi.fn();
    removeEmployerTeamMember.mockRejectedValueOnce(new Error('down')).mockResolvedValueOnce(undefined);
    render(EmployerTeam, { onLeft });
    const leave = async () => {
      await fireEvent.click(await screen.findByRole('button', { name: 'Leave the team' }));
      await fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Leave the team' }));
    };
    await leave();
    await vi.waitFor(() => expect(removeEmployerTeamMember).toHaveBeenCalledTimes(1));
    expect(removeEmployerTeamMember).toHaveBeenLastCalledWith(2);
    expect(onLeft).not.toHaveBeenCalled();
    await leave();
    await vi.waitFor(() => expect(onLeft).toHaveBeenCalledTimes(1));
    expect(removeEmployerTeamMember).toHaveBeenLastCalledWith(2);
  });
});
