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
vi.mock('#lib/api.js', () => ({
  ApiError: class extends Error {},
  api: { facetCounts: mock.counts },
}));

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
    await fireEvent.focus(screen.getByRole('combobox'));
    await fireEvent.mouseDown(await screen.findByRole('option', { name: 'Russian' }));
    await waitFor(() => expect(mock.update).toHaveBeenCalledWith(['en', 'ru']));
    await waitFor(() => expect(changed).toHaveBeenCalledOnce());
  });

  it('allows clearing a saved language even if it disappears from the catalogue', async () => {
    mock.counts.mockResolvedValue({ facets: { posting_language: { ru: 5 } } });
    render(JobLanguagesCard);
    const english = screen.getByRole('button', { name: 'Remove English' });
    await fireEvent.click(english);
    await waitFor(() => expect(mock.update).toHaveBeenCalledWith([]));
  });

  it('keeps a failed save from triggering alert synchronization', async () => {
    mock.update.mockRejectedValue(new Error('offline'));
    const changed = vi.fn();
    render(JobLanguagesCard, { onProfileChanged: changed });
    await fireEvent.focus(screen.getByRole('combobox'));
    await fireEvent.mouseDown(await screen.findByRole('option', { name: 'Russian' }));
    await screen.findByText(/Couldn.t save|Could not save/);
    expect(changed).not.toHaveBeenCalled();
  });

  it('names the field and language options in Russian', async () => {
    mock.locale = 'ru';
    render(JobLanguagesCard);
    expect(screen.getByRole('heading', { name: 'Языки вакансий' })).toBeTruthy();
    await fireEvent.focus(screen.getByRole('combobox'));
    expect(await screen.findByRole('option', { name: /русский/i })).toBeTruthy();
  });
});


describe('facet-backed language selector', () => {
  it('offers only available facet values and renders their flags', async () => {
    mock.counts.mockResolvedValue({ facets: { posting_language: { ja: 8, de: 0 } } });
    const { container } = render(JobLanguagesCard);
    expect(screen.queryByRole('listbox')).toBeNull();
    await fireEvent.focus(screen.getByRole('combobox'));
    const japanese = await screen.findByRole('option', { name: 'Japanese' });
    expect(japanese.querySelector('.fi-jp')).toBeTruthy();
    expect(screen.queryByRole('option', { name: 'German' })).toBeNull();
    expect(screen.queryByRole('option', { name: 'English' })).toBeNull();
    expect(container.querySelector('.fi-gb')).toBeTruthy();
    expect(mock.counts).toHaveBeenCalledWith(new URLSearchParams(), { facets: ['posting_language'] });
  });

  it('filters the dropdown and picks with the keyboard', async () => {
    render(JobLanguagesCard);
    const input = screen.getByRole('combobox');
    await fireEvent.focus(input);
    await screen.findByRole('option', { name: 'Russian' });
    await fireEvent.input(input, { target: { value: 'ru' } });
    expect(screen.queryByRole('option', { name: 'English' })).toBeNull();
    await fireEvent.keyDown(input, { key: 'ArrowDown' });
    await fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(mock.update).toHaveBeenCalledWith(['en', 'ru']));
    expect(screen.queryByRole('listbox')).toBeNull();
  });

  it('marks saved options as selected and dismisses an empty search with Escape', async () => {
    render(JobLanguagesCard);
    const input = screen.getByRole('combobox');
    await fireEvent.focus(input);
    expect((await screen.findByRole('option', { name: 'English' })).getAttribute('aria-selected')).toBe('true');
    expect(screen.getByRole('listbox').getAttribute('aria-multiselectable')).toBe('true');
    await fireEvent.input(input, { target: { value: 'zzzz' } });
    expect(screen.getByRole('option', { name: 'No languages found' }).getAttribute('aria-disabled')).toBe('true');
    await fireEvent.keyDown(input, { key: 'Escape' });
    expect(input.getAttribute('aria-expanded')).toBe('false');
    expect(mock.update).not.toHaveBeenCalled();
  });
});
