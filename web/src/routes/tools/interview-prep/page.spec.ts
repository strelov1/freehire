import { render, screen, within } from '@testing-library/svelte';
import { beforeEach, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api.js';
import type { InterviewPrepSheet } from '#lib/api-methods/jobs.js';
import PrepPage from './+page.svelte';

const { getInterviewPrep, currentUser } = vi.hoisted(() => ({
  getInterviewPrep: vi.fn(),
  currentUser: vi.fn(),
}));
vi.mock('$app/state', () => ({
  page: { data: {}, url: new URL('http://localhost/tools/interview-prep?job=backend-acme') },
}));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, assets: '' }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser }));
vi.mock('#lib/api.js', async (importOriginal) => ({
  ...await importOriginal<typeof import('#lib/api.js')>(),
  api: { getInterviewPrep },
}));

const posting = {
  job: { public_slug: 'backend-acme', title: 'Backend Engineer', company: 'Acme', location: 'Remote' },
  questions: [{ prompt: 'What trade-off did you make?', topic: 'Backend engineering' }],
} satisfies InterviewPrepSheet;

beforeEach(() => {
  currentUser.mockReset().mockReturnValue({ id: 7 });
  getInterviewPrep.mockReset();
});

it('uses only recorded details and prompts for missing STAR details', async () => {
  getInterviewPrep.mockResolvedValue({
    ...posting,
    examples: [{ claim: 'Migrated payments', context: 'A payments platform during a phased migration' }],
  } satisfies InterviewPrepSheet);
  render(PrepPage);

  expect(await screen.findByRole('heading', { name: 'Migrated payments' })).toBeTruthy();
  const example = within(screen.getByRole('region', { name: 'Examples for your answers' }));
  expect(example.queryByText('What was happening, and why did it matter?')).toBeNull();
  expect(example.getByText('What were you responsible for?')).toBeTruthy();
  expect(example.getByText('A payments platform during a phased migration')).toBeTruthy();
  expect(example.getByText('What steps did you personally take?')).toBeTruthy();
  expect(example.getByText('What changed, and how did you know?')).toBeTruthy();
});

it('keeps practice questions available alongside links to add or import evidence', async () => {
  getInterviewPrep.mockResolvedValue({ ...posting, examples: [] } satisfies InterviewPrepSheet);
  render(PrepPage);

  expect(await screen.findByText('What trade-off did you make?')).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Open Experience Bank' }).getAttribute('href')).toBe('/my/profile/experience');
  expect(screen.getByRole('link', { name: 'Upload a CV' }).getAttribute('href')).toBe('/my/profile');
});

it('keeps private evidence out of a guest sheet', async () => {
  currentUser.mockReturnValue(null);
  getInterviewPrep.mockResolvedValue({ ...posting, examples: [{ claim: 'Private example' }] } satisfies InterviewPrepSheet);
  render(PrepPage);

  expect(await screen.findByText('What trade-off did you make?')).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Sign in to find examples' })).toBeTruthy();
  expect(screen.queryByText('Private example')).toBeNull();
});

it('allows another search when the linked posting is unavailable', async () => {
  getInterviewPrep.mockRejectedValue(new ApiError(404, 'not found'));
  render(PrepPage);

  expect((await screen.findByRole('alert')).textContent).toContain('This job is no longer available');
  expect(screen.getByRole('searchbox', { name: 'Find a job' })).toBeTruthy();
});
