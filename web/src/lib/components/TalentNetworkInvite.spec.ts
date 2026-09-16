import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { TalentNetworkSetting } from '$lib/types';
import { TALENT_INVITE_DISMISSED_KEY } from '$lib/talentInvite';
import TalentNetworkInvite from './TalentNetworkInvite.svelte';

const { getTalentNetwork } = vi.hoisted(() => ({
  getTalentNetwork: vi.fn<() => Promise<TalentNetworkSetting>>(),
}));

vi.mock('$lib/api', () => ({ api: { getTalentNetwork } }));

beforeEach(() => {
  localStorage.clear();
  getTalentNetwork.mockReset().mockResolvedValue({
    talent_network_visibility: 'off',
    talent_handle: '',
    listed: false,
  });
});

describe('TalentNetworkInvite', () => {
  // No account is excluded any more — the beta gate that once hid this card is retired,
  // so there is nothing left to distinguish an account by.
  it('shows the invitation to every signed-in candidate', async () => {
    render(TalentNetworkInvite);

    await expect(screen.findByText('Get found without applying')).resolves.toBeTruthy();
  });

  // The card sits in the account shell now, above every `my/*` section — so it has to be
  // closable, and the choice has to survive the next navigation.
  it('hides itself when dismissed and remembers the choice', async () => {
    render(TalentNetworkInvite);
    await screen.findByText('Get found without applying');

    await fireEvent.click(screen.getByRole('button', { name: 'Hide this' }));

    await waitFor(() => expect(screen.queryByText('Get found without applying')).toBeNull());
    expect(localStorage.getItem(TALENT_INVITE_DISMISSED_KEY)).toBe('1');
  });

  // Asserted from the other side too: a stored dismissal must keep the card away on the
  // next render, which is the half a click-then-disappear test cannot see.
  it('stays hidden on a later visit, and asks the server nothing', async () => {
    localStorage.setItem(TALENT_INVITE_DISMISSED_KEY, '1');

    render(TalentNetworkInvite);

    // The card mounts on every `my/*` page, so a dismissed candidate who still fetched
    // would pay one request per account page load, for good, for something they will
    // never be shown. Waiting on a microtask turn first, so this asserts the request did
    // not happen rather than that it had not happened YET.
    await Promise.resolve();
    expect(getTalentNetwork).not.toHaveBeenCalled();
    expect(screen.queryByText('Get found without applying')).toBeNull();
  });
});
