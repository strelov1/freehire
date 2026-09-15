import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { TalentNetworkSetting } from '$lib/types';
import TalentNetworkInvite from './TalentNetworkInvite.svelte';

const { getTalentNetwork } = vi.hoisted(() => ({
  getTalentNetwork: vi.fn<() => Promise<TalentNetworkSetting>>(),
}));

vi.mock('$lib/api', () => ({ api: { getTalentNetwork } }));

beforeEach(() => {
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
});
