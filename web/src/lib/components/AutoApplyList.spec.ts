import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import AutoApplyList from './AutoApplyList.svelte';

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' } } }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params: { id?: string } = {}) => path.replace('[id]', params.id ?? ''),
}));
// Load the real primitives directly so this focused test does not compile the whole UI library.
vi.mock('#lib/ui/index.js', async () => ({
  Badge: (await import('freehire-design-system/badge.svelte')).default,
  Button: (await import('freehire-design-system/button.svelte')).default,
  EmptyState: (await import('freehire-design-system/empty-state.svelte')).default,
  Skeleton: (await import('freehire-design-system/skeleton.svelte')).default,
}));
vi.mock('@lucide/svelte', async () => ({
  ArrowRight: (await import('@lucide/svelte/icons/arrow-right')).default,
  RefreshCw: (await import('@lucide/svelte/icons/refresh-cw')).default,
}));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
const { listAutoApply } = vi.hoisted(() => ({ listAutoApply: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { listAutoApply } }));

beforeEach(() => listAutoApply.mockReset());

describe('Activity auto-apply list', () => {
  it('explains the empty state and the submission history window', async () => {
    listAutoApply.mockResolvedValue({ attempts: [] });
    render(AutoApplyList);
    expect(await screen.findByText('No automatic applications yet. Start one from a job page.')).toBeTruthy();
    expect(screen.getByText(/submissions from the last 30 days/)).toBeTruthy();
  });

  it('shows stopped and submitted applications with links to their tracking details', async () => {
    listAutoApply.mockResolvedValue({
      attempts: [
        {
          job_slug: 'engineer-abc',
          title: 'Software Engineer',
          company: 'Example',
          status: 'blocked',
          last_activity_at: '2026-11-13T12:00:00Z',
        },
        {
          job_slug: 'designer-def',
          title: 'Designer',
          company: 'Studio',
          status: 'submitted',
          last_activity_at: '2026-11-12T12:00:00Z',
        },
      ],
    });
    render(AutoApplyList);
    const link = await screen.findByRole('link', { name: /Software Engineer/ });
    expect(link.getAttribute('href')).toBe('/my/tracking/engineer-abc?tab=auto_apply');
    expect(screen.getByText('Needs attention')).toBeTruthy();
    expect(screen.getByText('Submitted')).toBeTruthy();
    expect(screen.getByText('Open the application to see what stopped it and what to do next.')).toBeTruthy();
  });

  it('lets the candidate retry a failed read', async () => {
    listAutoApply.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ attempts: [] });
    render(AutoApplyList);
    expect(await screen.findByText("Couldn't load your automatic applications.")).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('No automatic applications yet. Start one from a job page.')).toBeTruthy();
    expect(listAutoApply).toHaveBeenCalledTimes(2);
  });
});
