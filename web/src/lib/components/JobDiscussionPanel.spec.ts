import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, it, vi } from 'vitest';
import JobDiscussionPanel from './JobDiscussionPanel.svelte';

const { listThreads } = vi.hoisted(() => ({ listThreads: vi.fn() }));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/api.js', () => ({ api: { listThreads } }));

it('reads the discussion again when Retry follows a failed read', async () => {
  listThreads.mockRejectedValueOnce(new Error('down'));
  listThreads.mockResolvedValueOnce({ threads: [], nextCursor: undefined });
  render(JobDiscussionPanel, { slug: 'acme-job', active: true });

  await fireEvent.click(await screen.findByRole('button', { name: 'Retry' }));

  expect(await screen.findByText('No topics yet — be the first to start one.')).not.toBeNull();
  expect(listThreads).toHaveBeenCalledTimes(2);
});
