import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PipelineStats } from '$lib/types';
import PipelineView from './PipelineView.svelte';

const { getMyPipeline, user } = vi.hoisted(() => ({
  getMyPipeline: vi.fn(),
  user: { current: { id: 1 } as { id: number } | null },
}));

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/tracking/pipeline') } }));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$lib/api', () => ({ api: { getMyPipeline } }));
vi.mock('$lib/auth.svelte', () => ({
  currentUser: () => user.current,
  isAuthenticated: () => user.current !== null,
}));

const baseStats: PipelineStats = {
  applications: 12,
  stages: { applied: 12 } as PipelineStats['stages'],
};

beforeEach(() => {
  getMyPipeline.mockReset();
  user.current = { id: 1 };
});

describe('PipelineView reply-rate comparison card', () => {
  it('renders the comparison when reply_rate is present', async () => {
    getMyPipeline.mockResolvedValue({
      ...baseStats,
      reply_rate: { you: { applications: 12, answered: 4 }, global: { applications: 287, answered: 97 } },
    });

    render(PipelineView);

    await vi.waitFor(() => expect(screen.getByText('Your Reply Rate')).toBeTruthy());
    expect(screen.getByText('Average Reply Rate')).toBeTruthy();
  });

  it('renders no card and no placeholder when reply_rate is absent', async () => {
    getMyPipeline.mockResolvedValue(baseStats);

    render(PipelineView);

    // Wait for the ordinary content to appear first, so the assertion below is not
    // just "the fetch hasn't resolved yet".
    await vi.waitFor(() => expect(screen.getByText('Interview Rate')).toBeTruthy());
    expect(screen.queryByText('Your Reply Rate')).toBeNull();
    expect(screen.queryByText('Average Reply Rate')).toBeNull();
  });
});
