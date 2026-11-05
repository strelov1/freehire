import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import JobLanguagesCard from './JobLanguagesCard.svelte';

const mock = vi.hoisted(() => ({
  languages: ['en'] as string[],
  update: vi.fn(),
  counts: vi.fn(),
  locale: 'en',
}));
vi.mock('$app/state', () => ({ page: { get data() { return { locale: mock.locale }; } } }));
vi.mock('#lib/profile.svelte.js', () => ({ profileStore: {
  get profile() { return { job_languages: mock.languages }; },
  updateJobLanguages: mock.update,
} }));
vi.mock('#lib/api.js', () => ({ api: { facetCounts: mock.counts } }));

beforeEach(() => {
  mock.locale = 'en';
  mock.languages = ['en'];
  mock.update.mockReset().mockResolvedValue({});
  mock.counts.mockReset().mockResolvedValue({ facets: { posting_language: { en: 12, ru: 5 } } });
});
afterEach(cleanup);

describe('job language preferences', () => {
  it('adds another language and updates the profile-derived alert after saving', async () => {
    const changed = vi.fn();
    render(JobLanguagesCard, { onProfileChanged: changed });
    await fireEvent.click(await screen.findByRole('button', { name: 'Russian' }));
    await waitFor(() => expect(mock.update).toHaveBeenCalledWith(['en', 'ru']));
    await waitFor(() => expect(changed).toHaveBeenCalledOnce());
  });

  it('allows clearing a saved language even if it disappears from the catalogue', async () => {
    mock.counts.mockResolvedValue({ facets: { posting_language: { ru: 5 } } });
    render(JobLanguagesCard);
    const english = screen.getByRole('button', { name: 'English' });
    expect(english.getAttribute('aria-pressed')).toBe('true');
    await fireEvent.click(english);
    await waitFor(() => expect(mock.update).toHaveBeenCalledWith([]));
  });

  it('keeps a failed save from triggering alert synchronization', async () => {
    mock.update.mockRejectedValue(new Error('offline'));
    const changed = vi.fn();
    render(JobLanguagesCard, { onProfileChanged: changed });
    await fireEvent.click(await screen.findByRole('button', { name: 'Russian' }));
    await screen.findByText(/Couldn.t save|Could not save/);
    expect(changed).not.toHaveBeenCalled();
  });

  it('names the field and language options in Russian', async () => {
    mock.locale = 'ru';
    render(JobLanguagesCard);
    expect(screen.getByRole('heading', { name: 'Языки вакансий' })).toBeTruthy();
    expect(await screen.findByRole('button', { name: /русский/i })).toBeTruthy();
  });
});
