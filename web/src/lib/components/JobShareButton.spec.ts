import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, expect, it, vi } from 'vitest';
import JobShareButton from './JobShareButton.svelte';

vi.mock('$app/paths', () => ({
  resolve: (path: string, params: { slug: string }) => path.replace('[slug]', params.slug),
}));

const slug = 'backend-engineer-12345678';
const title = 'Backend Engineer';

afterEach(() => vi.unstubAllGlobals());

it('copies the public job link when native sharing is unavailable', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  render(JobShareButton, { slug, title });

  await fireEvent.click(screen.getByRole('button', { name: 'Share this job' }));
  expect(screen.queryByRole('menuitem', { name: 'Share' })).toBeNull();
  await fireEvent.click(screen.getByRole('menuitem', { name: 'Copy link' }));

  expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/jobs/backend-engineer-12345678`);
  expect(screen.getByRole('status').textContent).toBe('Link copied');
});

it('passes the job title and public link to native sharing', async () => {
  const share = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal('navigator', { share });
  render(JobShareButton, { slug, title });

  await fireEvent.click(screen.getByRole('button', { name: 'Share this job' }));
  await fireEvent.click(screen.getByRole('menuitem', { name: 'Share' }));

  expect(share).toHaveBeenCalledWith({
    title: 'Backend Engineer',
    url: `${window.location.origin}/jobs/backend-engineer-12345678`,
  });
  expect(screen.queryByRole('menu')).toBeNull();
});
