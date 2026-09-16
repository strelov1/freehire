import { render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { TalentNetworkSetting } from '$lib/types';
import TalentJoinButton from './TalentJoinButton.svelte';

const { getTalentNetwork, isAuthenticated } = vi.hoisted(() => ({
  getTalentNetwork: vi.fn<() => Promise<TalentNetworkSetting>>(),
  isAuthenticated: vi.fn<() => boolean>(),
}));

vi.mock('$lib/api', () => ({ api: { getTalentNetwork } }));
vi.mock('$lib/auth.svelte', () => ({ isAuthenticated }));

const setting = (visibility: TalentNetworkSetting['talent_network_visibility']) => ({
  talent_network_visibility: visibility,
  talent_handle: '',
  listed: false,
});

const joinLink = () => screen.findByRole('link', { name: /join the network/i });

beforeEach(() => {
  getTalentNetwork.mockReset().mockResolvedValue(setting('off'));
  isAuthenticated.mockReset().mockReturnValue(true);
});

describe('TalentJoinButton', () => {
  it('sends a signed-in non-member to the profile, where the decision is explained', async () => {
    render(TalentJoinButton);

    expect((await joinLink()).getAttribute('href')).toBe('/my/profile');
  });

  // The whole point of reading membership: a member seeing "Join" would be told something
  // untrue about their own account.
  it('shows nothing to a member', async () => {
    getTalentNetwork.mockResolvedValue(setting('anonymous'));

    render(TalentJoinButton);

    await waitFor(() => expect(getTalentNetwork).toHaveBeenCalled());
    expect(screen.queryByText(/join the network/i)).toBeNull();
  });

  // Signed out, membership cannot be read at all — so the control must not wait on a call
  // that would 401, and must still END on the profile rather than stranding the visitor
  // on whatever page /signin falls back to.
  it('routes a signed-out visitor through sign-in and on to the profile', async () => {
    isAuthenticated.mockReturnValue(false);

    render(TalentJoinButton);

    const href = (await joinLink()).getAttribute('href') ?? '';
    const url = new URL(href, 'https://freehire.me');
    expect(url.pathname).toBe('/signin');
    expect(url.searchParams.get('returnTo')).toBe('/my/profile');
    expect(getTalentNetwork).not.toHaveBeenCalled();
  });
});
